package backups

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestEncryptedBackupRoundTripAndTamperDetection(t *testing.T) {
	key, err := GenerateDataKey()
	if err != nil {
		t.Fatal(err)
	}
	plaintext := strings.Repeat("database-row\n", 10000)
	var encrypted bytes.Buffer
	checksum, size, err := CompressAndEncrypt(context.Background(), strings.NewReader(plaintext), &encrypted, key)
	if err != nil {
		t.Fatal(err)
	}
	if checksum == "" || size != int64(encrypted.Len()) || bytes.Contains(encrypted.Bytes(), []byte("database-row")) {
		t.Fatalf("backup encryption metadata invalid: checksum=%q size=%d", checksum, size)
	}
	var restored bytes.Buffer
	if err := DecryptAndDecompress(context.Background(), bytes.NewReader(encrypted.Bytes()), &restored, key); err != nil {
		t.Fatal(err)
	}
	if restored.String() != plaintext {
		t.Fatal("restored backup did not match source")
	}
	tampered := append([]byte(nil), encrypted.Bytes()...)
	tampered[len(tampered)-1] ^= 0xff
	if err := DecryptAndDecompress(context.Background(), bytes.NewReader(tampered), &bytes.Buffer{}, key); err == nil {
		t.Fatal("tampered encrypted backup was accepted")
	}
}
