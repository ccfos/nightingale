# [sorts](http://godoc.org/github.com/twotwotwo/sorts)

[sorts](http://godoc.org/github.com/twotwotwo/sorts) provides
parallel radix sorting by a string, []byte, or (u)int64 key, and a parallel
Quicksort(data). 
[sorts/sortutil](http://godoc.org/github.com/twotwotwo/sorts/sortutil)
sorts common slice types and adds functions to help sort floats.

Usually, stick to stdlib sort: that's fast, standard, and simpler.  But this
package may help if sorting huge datasets is a bottleneck for you.  To get a
sense of the potential gains, [some timings are available](https://docs.google.com/spreadsheets/d/1GkXMLXQ7oW5Bp0qwyYw0IiQElIq8B-IvNEYE_RPCTvA/edit#gid=0).

To radix sort, 
[implement sort.Interface](http://golang.org/pkg/sort/#Interface) 
plus one more method, Key(i int), returning the key for an item as
string/[]byte/(u)int64, and call sorts.ByString, ByBytes, ByUint64, or
ByInt64.  See the godoc for details and examples:
http://godoc.org/github.com/twotwotwo/sorts

There's no Reverse(), but sorts.Flip(data) will flip ascending-sorted
data to descending.  There's no stable sort.  The string sorts just compare
byte values; é won't sort next to e.  Set sorts.MaxProcs if you want to 
limit concurrency. The package checks that data is sorted after every run 
and panics(!) if not.

Credit (but no blame, or claim of endorsement) to the authors of stdlib sort; 
this uses its qSort, tests, and interface, and the clarity of its code 
helped make this possible.

I'd love to hear if you're using this. E-mail me at my github username at
GMail, or say hey on Twitter ([@rf](http://twitter.com/rf/)).
