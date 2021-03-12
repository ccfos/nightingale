package gosnmp

import "testing"

// GO SNMP credentials table
//nolint:gochecknoglobals,unused
var authenticationCredentials = map[string][]string{
	NoAuth.String() + NoPriv.String(): {"noAuthNoPrivUser", "", ""},

	MD5.String() + NoPriv.String(): {"authMD5OnlyUser", "testingpass0123456789", ""},
	MD5.String() + DES.String():    {"authMD5PrivDESUser", "testingpass9876543210", "testingpass9876543210"},
	MD5.String() + AES.String():    {"authMD5PrivAESUser", "AEStestingpass9876543210", "AEStestingpass9876543210"},
	// MD5.String() + AES192.String():		{ "authMD5PrivAES192BlmtUser", "authkey1", "privkey1" },
	// MD5.String() + AES192C.String():	{ "authMD5PrivAES192User", "authkey1", "privkey1" },
	// MD5.String() + AES256.String():		{ "authMD5PrivAES256BlmtUser", "authkey1", "privkey1" },
	// MD5.String() + AES256C.String():	{ "authMD5PrivAES256User", "authkey1", "privkey1" },

	SHA.String() + NoPriv.String(): {"authSHAOnlyUser", "testingpass9876543210", ""},
	SHA.String() + DES.String():    {"authSHAPrivDESUser", "testingpassabc6543210", "testingpassabc6543210"},
	SHA.String() + AES.String():    {"authSHAPrivAESUser", "AEStestingpassabc6543210", "AEStestingpassabc6543210"},
	// SHA.String() + AES192.String():		{ "authSHAPrivAES192BlmtUser", "authkey1", "privkey1" },
	// SHA.String() + AES192C.String():	{ "authSHAPrivAES192User", "authkey1", "privkey1" },
	// SHA.String() + AES256.String():		{ "authSHAPrivAES256BlmtUser", "authkey1", "privkey1" },
	// SHA.String() + AES256C.String():	{ "authSHAPrivAES256User", "authkey1", "privkey1" },

	SHA224.String() + NoPriv.String(): {"authSHA224OnlyUser", "testingpass5123456", ""},
	SHA224.String() + DES.String():    {"authSHA224PrivDESUser", "testingpass6123456", "testingpass6123456"},
	SHA224.String() + AES.String():    {"authSHA224PrivAESUser", "testingpass7123456", "testingpass7123456"},
	// SHA224.String() + AES192.String():	{ "authSHA224PrivAES192BlmtUser", "authkey1", "privkey1" },
	// SHA224.String() + AES192C.String():	{ "authSHA224PrivAES192User", "authkey1", "privkey1" },
	// SHA224.String() + AES256.String():	{ "authSHA224PrivAES256BlmtUser", "authkey1", "privkey1" },
	// SHA224.String() + AES256C.String():	{ "authSHA224PrivAES256User", "authkey1", "privkey1" },

	SHA256.String() + NoPriv.String(): {"authSHA256OnlyUser", "testingpass5223456", ""},
	SHA256.String() + DES.String():    {"authSHA256PrivDESUser", "testingpass6223456", "testingpass6223456"},
	SHA256.String() + AES.String():    {"authSHA256PrivAESUser", "testingpass7223456", "testingpass7223456"},
	// SHA256.String() + AES192.String():	{ "authSHA256PrivAES192BlmtUser", "authkey1", "privkey1" },
	// SHA256.String() + AES192C.String():	{ "authSHA256PrivAES192User", "authkey1", "privkey1" },
	// SHA256.String() + AES256.String():	{ "authSHA256PrivAES256BlmtUser", "authkey1", "privkey1" },
	// SHA256.String() + AES256C.String():	{ "authSHA256PrivAES256User", "authkey1", "privkey1" },

	SHA384.String() + NoPriv.String(): {"authSHA384OnlyUser", "testingpass5323456", ""},
	SHA384.String() + DES.String():    {"authSHA384PrivDESUser", "testingpass6323456", "testingpass6323456"},
	SHA384.String() + AES.String():    {"authSHA384PrivAESUser", "testingpass7323456", "testingpass7323456"},
	// SHA384.String() + AES192.String():	{ "authSHA384PrivAES192BlmtUser", "authkey1", "privkey1" },
	// SHA384.String() + AES192C.String():	{ "authSHA384PrivAES192User", "authkey1", "privkey1" },
	// SHA384.String() + AES256.String():	{ "authSHA384PrivAES256BlmtUser", "authkey1", "privkey1" },
	// SHA384.String() + AES256C.String():	{ "authSHA384PrivAES256User", "authkey1", "privkey1" },

	SHA512.String() + NoPriv.String(): {"authSHA512OnlyUser", "testingpass5423456", ""},
	SHA512.String() + DES.String():    {"authSHA512PrivDESUser", "testingpass6423456", "testingpass6423456"},
	SHA512.String() + AES.String():    {"authSHA512PrivAESUser", "testingpass7423456", "testingpass7423456"},
	// SHA512.String() + AES192.String():	{ "authSHA512PrivAES192BlmtUser", "authkey1", "privkey1" },
	// SHA512.String() + AES192C.String():	{ "authSHA512PrivAES192User", "authkey1", "privkey1" },
	// SHA512.String() + AES256.String():	{ "authSHA512PrivAES256BlmtUser", "authkey1", "privkey1" },
	// SHA512.String() + AES256C.String():	{ "authSHA512PrivAES256User", "authkey1", "privkey1" },
}

// Credentials table for public demo.snmplabs.org
//nolint:unused,gochecknoglobals
var authenticationCredentialsSnmpLabs = map[string][]string{
	NoAuth.String() + NoPriv.String(): {"usr-none-none", "", ""},

	MD5.String() + NoPriv.String():  {"usr-md5-none", "authkey1", ""},
	MD5.String() + DES.String():     {"usr-md5-des", "authkey1", "privkey1"},
	MD5.String() + AES.String():     {"usr-md5-aes", "authkey1", "privkey1"},
	MD5.String() + AES192.String():  {"usr-md5-aes192-blmt", "authkey1", "privkey1"},
	MD5.String() + AES192C.String(): {"usr-md5-aes192", "authkey1", "privkey1"},
	MD5.String() + AES256.String():  {"usr-md5-aes256-blmt", "authkey1", "privkey1"},
	MD5.String() + AES256C.String(): {"usr-md5-aes256", "authkey1", "privkey1"},

	SHA.String() + NoPriv.String():  {"usr-sha-none", "authkey1", ""},
	SHA.String() + DES.String():     {"usr-sha-des", "authkey1", "privkey1"},
	SHA.String() + AES.String():     {"usr-sha-aes", "authkey1", "privkey1"},
	SHA.String() + AES192.String():  {"usr-sha-aes192-blmt", "authkey1", "privkey1"},
	SHA.String() + AES192C.String(): {"usr-sha-aes192", "authkey1", "privkey1"},
	SHA.String() + AES256.String():  {"usr-sha-aes256-blmt", "authkey1", "privkey1"},
	SHA.String() + AES256C.String(): {"usr-sha-aes256", "authkey1", "privkey1"},

	SHA224.String() + NoPriv.String():  {"usr-sha224-none", "authkey1", ""},
	SHA224.String() + DES.String():     {"usr-sha224-des", "authkey1", "privkey1"},
	SHA224.String() + AES.String():     {"usr-sha224-aes", "authkey1", "privkey1"},
	SHA224.String() + AES192.String():  {"usr-sha224-aes192-blmt", "authkey1", "privkey1"},
	SHA224.String() + AES192C.String(): {"usr-sha224-aes192", "authkey1", "privkey1"},
	SHA224.String() + AES256.String():  {"usr-sha224-aes256-blmt", "authkey1", "privkey1"},
	SHA224.String() + AES256C.String(): {"usr-sha224-aes256", "authkey1", "privkey1"},

	SHA256.String() + NoPriv.String():  {"usr-sha256-none", "authkey1", ""},
	SHA256.String() + DES.String():     {"usr-sha256-des", "authkey1", "privkey1"},
	SHA256.String() + AES.String():     {"usr-sha256-aes", "authkey1", "privkey1"},
	SHA256.String() + AES192.String():  {"usr-sha256-aes192-blmt", "authkey1", "privkey1"},
	SHA256.String() + AES192C.String(): {"usr-sha256-aes192", "authkey1", "privkey1"},
	SHA256.String() + AES256.String():  {"usr-sha256-aes256-blmt", "authkey1", "privkey1"},
	SHA256.String() + AES256C.String(): {"usr-sha256-aes256", "authkey1", "privkey1"},

	SHA384.String() + NoPriv.String():  {"usr-sha384-none", "authkey1", ""},
	SHA384.String() + DES.String():     {"usr-sha384-des", "authkey1", "privkey1"},
	SHA384.String() + AES.String():     {"usr-sha384-aes", "authkey1", "privkey1"},
	SHA384.String() + AES192.String():  {"usr-sha384-aes192-blmt", "authkey1", "privkey1"},
	SHA384.String() + AES192C.String(): {"usr-sha384-aes192", "authkey1", "privkey1"},
	SHA384.String() + AES256.String():  {"usr-sha384-aes256-blmt", "authkey1", "privkey1"},
	SHA384.String() + AES256C.String(): {"usr-sha384-aes256", "authkey1", "privkey1"},

	SHA512.String() + NoPriv.String():  {"usr-sha512-none", "authkey1", ""},
	SHA512.String() + DES.String():     {"usr-sha512-des", "authkey1", "privkey1"},
	SHA512.String() + AES.String():     {"usr-sha512-aes", "authkey1", "privkey1"},
	SHA512.String() + AES192.String():  {"usr-sha512-aes192-blmt", "authkey1", "privkey1"},
	SHA512.String() + AES192C.String(): {"usr-sha512-aes192", "authkey1", "privkey1"},
	SHA512.String() + AES256.String():  {"usr-sha512-aes256-blmt", "authkey1", "privkey1"},
	SHA512.String() + AES256C.String(): {"usr-sha512-aes256", "authkey1", "privkey1"},
}

//nolint:unused,gochecknoglobals
var useSnmpLabsCredentials = false

// TODO get above credentials into snmpsimd, so *all* tests can be run. Combine with settings in `snmp_users.sh`

const cIdxUserName = 0
const cIdxAuthKey = 1
const cIdxPrivKey = 2

//nolint
func isUsingSnmpLabs() bool {
	return useSnmpLabsCredentials
}

// conveniently enable demo.snmplabs.com for a one test
//nolint
func useSnmpLabs(use bool) {
	useSnmpLabsCredentials = use
}

//nolint
func getCredentials(t *testing.T, authProtocol SnmpV3AuthProtocol, privProtocol SnmpV3PrivProtocol) []string {
	var credentials []string
	if useSnmpLabsCredentials {
		credentials = authenticationCredentialsSnmpLabs[authProtocol.String()+privProtocol.String()]
	} else {
		credentials = authenticationCredentials[authProtocol.String()+privProtocol.String()]
	}

	if credentials == nil {
		t.Skipf("No user credentials found for %s/%s", authProtocol.String(), privProtocol.String())
		return []string{"unknown", "unknown", "unkown"}
	}
	return credentials
}

//nolint
func getUserName(t *testing.T, authProtocol SnmpV3AuthProtocol, privProtocol SnmpV3PrivProtocol) string {
	return getCredentials(t, authProtocol, privProtocol)[cIdxUserName]
}

//nolint:unused,deadcode
func getAuthKey(t *testing.T, authProtocol SnmpV3AuthProtocol, privProtocol SnmpV3PrivProtocol) string {
	return getCredentials(t, authProtocol, privProtocol)[cIdxAuthKey]
}

//nolint:unused,deadcode
func getPrivKey(t *testing.T, authProtocol SnmpV3AuthProtocol, privProtocol SnmpV3PrivProtocol) string {
	return getCredentials(t, authProtocol, privProtocol)[cIdxPrivKey]
}
