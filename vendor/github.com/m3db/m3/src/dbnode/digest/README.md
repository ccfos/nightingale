# digest

This package consolidates all our digest algorithms used for data integrity into a single place.  Adler32 is used and dependendent on the use case we rely on the standard library or a modified rolling hash version that can be stack allocated.

For highly concurrent callsites that require digests, they use the stack based adler32 library to avoid having to pool digest structs.  The stack based adler32 library is a few percent slower than the standard library adler32 algorithm but heap allocation free.

For less concurrent callsites or callsites already under a mutex a cached digest struct can be kept and reused, these callsites use the standard library methods.

For callsites that do not need to incremental checksumming, meaning they only need to checksum a single byte slice, the static checksum method from the standard library is used.
