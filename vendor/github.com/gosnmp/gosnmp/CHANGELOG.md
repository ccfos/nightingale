## unreleased

* [CHANGE]
* [FEATURE]
* [ENHANCEMENT]
* [BUGFIX]

## v1.29.0

NOTE: This release returns the OctetString []byte behavior for v1.26.0 and earlier.

* [CHANGE] Return OctetString as []byte #264

## v1.28.0

This release updates the Go import path from `github.com/soniah/gosnmp`
to `github.com/gosnmp/gosnmp`.

* [CHANGE] Update project path #257
* [ENHANCEMENT] Improve SNMPv3 trap support #253

## v1.27.0

* fix a race condition - logger
* INFORM responses
* linting

## v1.26.0

* more SNMPv3
* various bug fixes
* linting

## v1.25.0

* SNMPv3 new hash functions for SNMPV3 USM RFC7860
* SNMPv3 tests for SNMPv3 traps
* go versions 1.12 1.13

## v1.24.0

* doco, fix AUTHORS, fix copyright
* decode more packet types
* TCP trap listening

## v1.23.1

* add support for contexts
* fix panic conditions by checking for out-of-bounds reads

## v1.23.0

* BREAKING CHANGE: The mocks have been moved to `github.com/gosnmp/gosnmp/mocks`.
  If you use them, you will need to adjust your imports.
* bug fix: issue 170: No results when performing a walk starting on a leaf OID
* bug fix: issue 210: Set function fails if value is an Integer
* doco: loggingEnabled, MIB parser
* linting

## v1.22.0

* travis now failing build when goimports needs running
* gometalinter
* shell script for running local tests
* SNMPv3 - avoid crash when missing SecurityParameters
* add support for Walk and Get over TCP - RFC 3430
* SNMPv3 - allow input of private key instead of passphrase

## v1.21.0

* add netsnmp functionality "not check returned OIDs are increasing"

## v1.20.0

* convert all tags to correct semantic versioning, and remove old tags
* SNMPv1 trap IDs should be marshalInt32() not single byte
* use packetSecParams not sp secretKey in v3 isAuthentic()
* fix IPAddress marshalling in Set()

## v1.19.0

* bug fix: handle uninitialized v3 SecurityParameters in SnmpDecodePacket()
* SNMPError, Asn1BER - stringers; types on constants

## v1.18.0

* bug fix: use format flags - logPrintf() not logPrint()
* bug fix: parseObjectIdentifier() now returns []byte{0} rather than error
  when it receive zero length input
* use gomock
* start using go modules
* start a changelog
