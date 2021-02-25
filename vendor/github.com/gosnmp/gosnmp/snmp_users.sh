#!/bin/bash

cat << EOF >> /etc/snmp/snmpd.conf
createUser noAuthNoPrivUser
createUser authMD5OnlyUser  MD5 testingpass0123456789
createUser authSHAOnlyUser  SHA testingpass9876543210
createUser authSHA224OnlyUser SHA224 testingpass5123456
createUser authSHA256OnlyUser SHA256 testingpass5223456
createUser authSHA384OnlyUser SHA384 testingpass5323456
createUser authSHA512OnlyUser SHA512 testingpass5423456

createUser authMD5PrivDESUser MD5 testingpass9876543210 DES
createUser authSHAPrivDESUser SHA testingpassabc6543210 DES
createUser authSHA224PrivDESUser SHA224 testingpass6123456 DES
createUser authSHA256PrivDESUser SHA256 testingpass6223456 DES
createUser authSHA384PrivDESUser SHA384 testingpass6323456 DES
createUser authSHA512PrivDESUser SHA512 testingpass6423456 DES

createUser authMD5PrivAESUser MD5 AEStestingpass9876543210 AES
createUser authSHAPrivAESUser SHA AEStestingpassabc6543210 AES
createUser authSHA224PrivAESUser SHA224 testingpass7123456 AES
createUser authSHA256PrivAESUser SHA256 testingpass7223456 AES
createUser authSHA384PrivAESUser SHA384 testingpass7323456 AES
createUser authSHA512PrivAESUser SHA512 testingpass7423456 AES

rouser   noAuthNoPrivUser noauth
rouser   authMD5OnlyUser auth
rouser   authSHAOnlyUser auth
rouser   authSHA224OnlyUser auth
rouser   authSHA256OnlyUser auth
rouser   authSHA384OnlyUser auth
rouser   authSHA512OnlyUser auth

rouser   authMD5PrivDESUser authPriv
rouser   authSHAPrivDESUser authPriv
rouser   authSHA224PrivDESUser authPriv
rouser   authSHA256PrivDESUser authPriv
rouser   authSHA384PrivDESUser authPriv
rouser   authSHA512PrivDESUser authPriv

rouser   authMD5PrivAESUser authPriv
rouser   authSHAPrivAESUser authPriv
rouser   authSHA224PrivAESUser authPriv
rouser   authSHA256PrivAESUser authPriv
rouser   authSHA384PrivAESUser authPriv
rouser   authSHA512PrivAESUser authPriv
EOF

# enable ipv6 TODO restart fails - need to enable ipv6 on interface; spin up a Linux instance to check this
# sed -i -e '/agentAddress/ s/^/#/' -e '/agentAddress/ s/^##//' /etc/snmp/snmpd.conf
