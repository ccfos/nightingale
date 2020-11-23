#!/bin/sh

VERSION=$(cat ./VERSION)

cat > version.go <<EOF
package lightstep

// TracerVersionValue provides the current version of the lightstep-tracer-go release
const TracerVersionValue = "$VERSION"
EOF
