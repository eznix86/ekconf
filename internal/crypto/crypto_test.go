package crypto

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEncryptDecrypt_RoundTrip(t *testing.T) {
	tests := []struct {
		name      string
		password  string
		plaintext string
	}{
		{
			name:      "simple password and text",
			password:  "hunter2",
			plaintext: "apiVersion: v1\nkind: Config\ncurrent-context: prod\n",
		},
		{
			name:      "empty plaintext",
			password:  "password123",
			plaintext: "",
		},
		{
			name:      "long password",
			password:  "this is a very long password with spaces and $pecial chars!",
			plaintext: "some config data here",
		},
		{
			name:      "unicode content",
			password:  "pässwörd",
			plaintext: "context: produção\ncluster: produção-01\n",
		},
		{
			name:      "large payload",
			password:  "test",
			plaintext: string(make([]byte, 10000)),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ef, err := Encrypt([]byte(tt.plaintext), tt.password)
			require.NoError(t, err)
			require.NotNil(t, ef)

			assert.Len(t, ef.Salt, SaltLen)
			assert.Len(t, ef.Nonce, NonceLen)
			assert.NotEmpty(t, ef.Ciphertext)

			decrypted, err := Decrypt(ef, tt.password)
			require.NoError(t, err)
			assert.Equal(t, tt.plaintext, string(decrypted))
		})
	}
}

func TestEncrypt_UniqueSaltAndNonce(t *testing.T) {
	a, err := Encrypt([]byte("hello"), "pass")
	require.NoError(t, err)

	b, err := Encrypt([]byte("hello"), "pass")
	require.NoError(t, err)

	assert.NotEqual(t, a.Salt, b.Salt, "salt should be unique per encryption")
	assert.NotEqual(t, a.Nonce, b.Nonce, "nonce should be unique per encryption")
	assert.NotEqual(t, a.Ciphertext, b.Ciphertext, "ciphertext should be unique per encryption (different salt/nonce)")
}

func TestDecrypt_WrongPassword(t *testing.T) {
	ef, err := Encrypt([]byte("secret data"), "correct-password")
	require.NoError(t, err)

	_, err = Decrypt(ef, "wrong-password")
	require.Error(t, err)
	assert.ErrorContains(t, err, "decrypt")
}

func TestDecrypt_TamperedCiphertext(t *testing.T) {
	ef, err := Encrypt([]byte("secret data"), "password")
	require.NoError(t, err)

	ef.Ciphertext[0] ^= 0xFF

	_, err = Decrypt(ef, "password")
	require.Error(t, err)
	assert.ErrorContains(t, err, "decrypt")
}

func TestMarshalUnmarshal_RoundTrip(t *testing.T) {
	ef, err := Encrypt([]byte("hello world"), "password")
	require.NoError(t, err)

	data := Marshal(ef)
	require.NotEmpty(t, data)

	restored, err := Unmarshal(data)
	require.NoError(t, err)

	assert.Equal(t, ef.Salt, restored.Salt)
	assert.Equal(t, ef.Nonce, restored.Nonce)
	assert.Equal(t, ef.Ciphertext, restored.Ciphertext)

	decrypted, err := Decrypt(restored, "password")
	require.NoError(t, err)
	assert.Equal(t, "hello world", string(decrypted))
}

func TestUnmarshal_ShortData(t *testing.T) {
	tests := []struct {
		name string
		data []byte
	}{
		{name: "empty", data: nil},
		{name: "too short", data: make([]byte, SaltLen+NonceLen-1)},
		{name: "only salt", data: make([]byte, SaltLen)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Unmarshal(tt.data)
			require.Error(t, err)
			assert.ErrorContains(t, err, "too short")
		})
	}
}

func TestMarshal_Format(t *testing.T) {
	ef, err := Encrypt([]byte("data"), "pass")
	require.NoError(t, err)

	data := Marshal(ef)

	assert.Len(t, data, SaltLen+NonceLen+len(ef.Ciphertext))
	assert.Equal(t, ef.Salt, data[:SaltLen])
	assert.Equal(t, ef.Nonce, data[SaltLen:SaltLen+NonceLen])
	assert.Equal(t, ef.Ciphertext, data[SaltLen+NonceLen:])
}

func TestSealOpen_RoundTrip(t *testing.T) {
	blob, err := Seal([]byte("secret data"), "password")
	require.NoError(t, err)
	require.True(t, IsSecretBox(blob))

	decrypted, err := Open(blob, "password")
	require.NoError(t, err)
	assert.Equal(t, "secret data", string(decrypted))
}

func TestOpen_LegacyFormat(t *testing.T) {
	ef, err := Encrypt([]byte("legacy data"), "password")
	require.NoError(t, err)
	data := Marshal(ef)
	require.False(t, IsSecretBox(data))

	decrypted, err := Open(data, "password")
	require.NoError(t, err)
	assert.Equal(t, "legacy data", string(decrypted))
}

func TestOpen_WrongPassword(t *testing.T) {
	blob, err := Seal([]byte("secret data"), "correct-password")
	require.NoError(t, err)

	_, err = Open(blob, "wrong-password")
	require.Error(t, err)
	assert.ErrorContains(t, err, "unseal")
}

func TestMigrate_LegacyFormat(t *testing.T) {
	ef, err := Encrypt([]byte("legacy data"), "password")
	require.NoError(t, err)

	migratedData, migrated, err := Migrate(Marshal(ef), "password")
	require.NoError(t, err)
	require.True(t, migrated)
	require.True(t, IsSecretBox(migratedData))

	decrypted, err := Open(migratedData, "password")
	require.NoError(t, err)
	assert.Equal(t, "legacy data", string(decrypted))
}

func TestMigrate_CurrentFormatNoop(t *testing.T) {
	blob, err := Seal([]byte("secret data"), "password")
	require.NoError(t, err)

	migratedData, migrated, err := Migrate(blob, "password")
	require.NoError(t, err)
	assert.False(t, migrated)
	assert.Nil(t, migratedData)
}

func TestDecrypt_EmptyPassword(t *testing.T) {
	ef, err := Encrypt([]byte("data"), "")
	require.NoError(t, err)

	decrypted, err := Decrypt(ef, "")
	require.NoError(t, err)
	assert.Equal(t, "data", string(decrypted))

	_, err = Decrypt(ef, "not-empty")
	assert.Error(t, err)
}
