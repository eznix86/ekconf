package crypto

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEncrypt_DecryptMultipleContexts(t *testing.T) {
	plaintexts := []string{
		"config for prod cluster with many secrets",
		"config for staging cluster",
		"config for dev cluster with special chars: !@#$%^&*()",
	}
	password := "the-password"

	for i, pt := range plaintexts {
		t.Run(string(rune('a'+i)), func(t *testing.T) {
			ef, err := Encrypt([]byte(pt), password)
			require.NoError(t, err)

			decrypted, err := Decrypt(ef, password)
			require.NoError(t, err)
			assert.Equal(t, pt, string(decrypted))
		})
	}
}

func TestEncrypt_DifferentPasswords(t *testing.T) {
	data := []byte("same data")
	ef1, err := Encrypt(data, "pass1")
	require.NoError(t, err)

	ef2, err := Encrypt(data, "pass2")
	require.NoError(t, err)

	_, err = Decrypt(ef1, "pass2")
	assert.Error(t, err)

	_, err = Decrypt(ef2, "pass1")
	assert.Error(t, err)

	d1, err := Decrypt(ef1, "pass1")
	require.NoError(t, err)
	assert.Equal(t, data, d1)

	d2, err := Decrypt(ef2, "pass2")
	require.NoError(t, err)
	assert.Equal(t, data, d2)
}

func TestDeriveKey_Consistent(t *testing.T) {
	salt := []byte("0123456789abcdef")
	k1 := deriveKey("password", salt)
	k2 := deriveKey("password", salt)

	assert.Equal(t, k1, k2)
	assert.Len(t, k1, ArgonKeyLen)
}

func TestDeriveKey_DifferentSalt(t *testing.T) {
	k1 := deriveKey("password", []byte("0123456789abcdef"))
	k2 := deriveKey("password", []byte("fedcba9876543210"))

	assert.NotEqual(t, k1, k2)
}

func TestDeriveKey_DifferentPassword(t *testing.T) {
	salt := []byte("0123456789abcdef")
	k1 := deriveKey("password1", salt)
	k2 := deriveKey("password2", salt)

	assert.NotEqual(t, k1, k2)
}
