package shortener

// alphabet: 0-9, A-Z, a-z — 62 characters, isliye "base62"
const alphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

const base = uint64(len(alphabet))

// Encode counter number ko chhote alphanumeric code mein badalta hai
// (jaise base10 -> base62), taaki URL ka code jitna ho sake chhota rahe.
func Encode(n uint64) string {
	if n == 0 {
		return string(alphabet[0])
	}

	var buf []byte
	for n > 0 {
		remainder := n % base
		buf = append(buf, alphabet[remainder])
		n /= base
	}

	// digits reverse order mein nikalte hain, isliye palatna padta hai
	for i, j := 0, len(buf)-1; i < j; i, j = i+1, j-1 {
		buf[i], buf[j] = buf[j], buf[i]
	}
	return string(buf)
}
