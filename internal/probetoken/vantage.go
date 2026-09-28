package probetoken

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"io"
)

// VantageIDPrefix starts every vantage ID.
const VantageIDPrefix = "vtg_"

const (
	vantageNonceLen = 16
	vantageTagLen   = 16
	// vantageIDLen is the prefix plus lowercase hex of nonce and tag.
	vantageIDLen = len(VantageIDPrefix) + 2*(vantageNonceLen+vantageTagLen)
)

// NewVantageID mints a vantage ID for orgID: VantageIDPrefix followed by
// 32 lowercase hex characters of random nonce and 32 of an HMAC-SHA256 tag
// over (orgID, nonce) under the key's binding sub-key.
//
// The tag lets the server recognise, with no stored state, that an ID was
// minted by this key for this organization. Rotation therefore keeps a vantage
// stable without trusting the ID a caller supplies, and an ID minted for one
// organization cannot be re-used under another.
func (k Key) NewVantageID(orgID string, random io.Reader) (string, error) {
	if !k.Configured() {
		return "", ErrKeyNotConfigured
	}
	if !validOrgID(orgID) {
		return "", ErrOrg
	}
	if random == nil {
		return "", ErrRandom
	}
	nonce := make([]byte, vantageNonceLen)
	if _, err := io.ReadFull(random, nonce); err != nil {
		return "", ErrRandom
	}
	tag := k.vantageTag(orgID, nonce)
	return VantageIDPrefix + hex.EncodeToString(nonce) + hex.EncodeToString(tag), nil
}

// VantageIDBoundTo reports whether id is a well-formed vantage ID minted by
// this key for orgID. The tag comparison is constant-time. It is false when
// the key is not configured.
func (k Key) VantageIDBoundTo(id, orgID string) bool {
	if !k.Configured() || !validOrgID(orgID) {
		return false
	}
	nonce, tag, ok := splitVantageID(id)
	if !ok {
		return false
	}
	return hmac.Equal(tag, k.vantageTag(orgID, nonce))
}

// vantageTag is HMAC-SHA256(bind, len(orgID) || orgID || nonce), truncated.
// The length prefix keeps (orgID, nonce) pairs unambiguous.
func (k Key) vantageTag(orgID string, nonce []byte) []byte {
	mac := hmac.New(sha256.New, k.m.bind)
	var n [8]byte
	binary.BigEndian.PutUint64(n[:], uint64(len(orgID)))
	mac.Write(n[:])
	mac.Write([]byte(orgID))
	mac.Write(nonce)
	return mac.Sum(nil)[:vantageTagLen]
}

// splitVantageID accepts only the canonical form: the prefix followed by
// exactly 64 lowercase hex characters. Upper-case hex is rejected so that one
// vantage has exactly one spelling.
func splitVantageID(id string) (nonce, tag []byte, ok bool) {
	if len(id) != vantageIDLen || id[:len(VantageIDPrefix)] != VantageIDPrefix {
		return nil, nil, false
	}
	body := id[len(VantageIDPrefix):]
	for i := 0; i < len(body); i++ {
		c := body[i]
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return nil, nil, false
		}
	}
	raw, err := hex.DecodeString(body)
	if err != nil {
		return nil, nil, false
	}
	return raw[:vantageNonceLen], raw[vantageNonceLen:], true
}
