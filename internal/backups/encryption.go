package backups

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
)

var encryptedBackupMagic = []byte("HFBK1\x00")

type countingWriter struct {
	w io.Writer
	n int64
}

func DecryptAndDecompress(ctx context.Context, source io.Reader, destination io.Writer, key []byte) error {
	block, err := aes.NewCipher(key)
	if err != nil {
		return err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return err
	}
	header := make([]byte, len(encryptedBackupMagic))
	if _, err := io.ReadFull(source, header); err != nil || !bytes.Equal(header, encryptedBackupMagic) {
		return fmt.Errorf("invalid HostForge backup header")
	}
	pipeReader, pipeWriter := io.Pipe()
	go func() {
		for {
			var length [4]byte
			if _, err := io.ReadFull(source, length[:]); err != nil {
				if err == io.EOF {
					_ = pipeWriter.Close()
				} else {
					_ = pipeWriter.CloseWithError(err)
				}
				return
			}
			nonce := make([]byte, aead.NonceSize())
			if _, err := io.ReadFull(source, nonce); err != nil {
				_ = pipeWriter.CloseWithError(err)
				return
			}
			ciphertext := make([]byte, binary.BigEndian.Uint32(length[:]))
			if _, err := io.ReadFull(source, ciphertext); err != nil {
				_ = pipeWriter.CloseWithError(err)
				return
			}
			plaintext, err := aead.Open(nil, nonce, ciphertext, nil)
			if err != nil {
				_ = pipeWriter.CloseWithError(fmt.Errorf("decrypt backup chunk: %w", err))
				return
			}
			if _, err := pipeWriter.Write(plaintext); err != nil {
				return
			}
		}
	}()
	compressed, err := gzip.NewReader(pipeReader)
	if err != nil {
		return err
	}
	defer compressed.Close()
	_, err = io.Copy(destination, &contextReader{ctx: ctx, reader: compressed})
	return err
}

func (w *countingWriter) Write(value []byte) (int, error) {
	n, err := w.w.Write(value)
	w.n += int64(n)
	return n, err
}

type chunkEncryptWriter struct {
	w    io.Writer
	aead cipher.AEAD
}

func (w *chunkEncryptWriter) Write(value []byte) (int, error) {
	if len(value) == 0 {
		return 0, nil
	}
	nonce := make([]byte, w.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return 0, err
	}
	ciphertext := w.aead.Seal(nil, nonce, value, nil)
	if len(ciphertext) > int(^uint32(0)) {
		return 0, fmt.Errorf("encrypted backup chunk too large")
	}
	var length [4]byte
	binary.BigEndian.PutUint32(length[:], uint32(len(ciphertext)))
	if _, err := w.w.Write(length[:]); err != nil {
		return 0, err
	}
	if _, err := w.w.Write(nonce); err != nil {
		return 0, err
	}
	if _, err := w.w.Write(ciphertext); err != nil {
		return 0, err
	}
	return len(value), nil
}

func GenerateDataKey() ([]byte, error) {
	key := make([]byte, 32)
	_, err := rand.Read(key)
	return key, err
}

// CompressAndEncrypt streams source through gzip and chunked AES-256-GCM.
// The returned checksum and size describe the encrypted object bytes.
func CompressAndEncrypt(ctx context.Context, source io.Reader, destination io.Writer, key []byte) (string, int64, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", 0, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return "", 0, err
	}
	hash := sha256.New()
	counted := &countingWriter{w: io.MultiWriter(destination, hash)}
	if _, err := counted.Write(encryptedBackupMagic); err != nil {
		return "", counted.n, err
	}
	encrypted := &chunkEncryptWriter{w: counted, aead: aead}
	compressed := gzip.NewWriter(encrypted)
	_, copyErr := io.Copy(compressed, &contextReader{ctx: ctx, reader: source})
	closeErr := compressed.Close()
	if copyErr != nil {
		return "", counted.n, copyErr
	}
	if closeErr != nil {
		return "", counted.n, closeErr
	}
	return hex.EncodeToString(hash.Sum(nil)), counted.n, nil
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r *contextReader) Read(value []byte) (int, error) {
	select {
	case <-r.ctx.Done():
		return 0, r.ctx.Err()
	default:
		return r.reader.Read(value)
	}
}
