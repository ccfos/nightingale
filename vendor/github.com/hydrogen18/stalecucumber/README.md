#stalecucumber

This package reads and writes pickled data. The format is the same
as the Python "pickle" module.

Protocols 0,1,2 are implemented. These are the versions written by the Python
2.x series. Python 3 defines newer protocol versions, but can write the older
protocol versions so they are readable by this package.

[Full documentation is available here.](https://godoc.org/github.com/hydrogen18/stalecucumber)

##TLDR

Read a pickled string or unicode object
```
pickle.dumps("foobar")
---
var somePickledData io.Reader
mystring, err := stalecucumber.String(stalecucumber.Unpickle(somePickledData))
````

Read a pickled integer
```
pickle.dumps(42)
---
var somePickledData io.Reader
myint64, err := stalecucumber.Int(stalecucumber.Unpickle(somePickledData))
```

Read a pickled list of numbers into a structure
```
pickle.dumps([8,8,2005])
---
var somePickledData io.Reader
numbers := make([]int64,0)

err := stalecucumber.UnpackInto(&numbers).From(stalecucumber.Unpickle(somePickledData))
```

Read a pickled dictionary into a structure
```
pickle.dumps({"apple":1,"banana":2,"cat":"hello","Dog":42.0})
---
var somePickledData io.Reader
mystruct := struct{
	Apple int
	Banana uint
	Cat string
	Dog float32}{}

err := stalecucumber.UnpackInto(&mystruct).From(stalecucumber.Unpickle(somePickledData))
```

Pickle a struct

```
buf := new(bytes.Buffer)
mystruct := struct{
		Apple int
		Banana uint
		Cat string
		Dog float32}{}

err := stalecucumber.NewPickler(buf).Pickle(mystruct)
```
