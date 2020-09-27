## cache from revel

demo

```go
package main

import (
    "github.com/toolkits/cache"
    "log"
    "time"
)

func main() {
    log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)
    cache.InitCache(
        "127.0.0.1:6379",
        5,
        10,
        time.Minute,
        time.Minute,
        time.Minute,
        time.Hour,
    )

    var name string
    if err := cache.Get("name", &name); err != nil {
        log.Println("not found name, set it")
        cache.Set("name", "Ulric", time.Second)
    } else {
        log.Println("should not be in here")
    }

    if err := cache.Get("name", &name); err != nil {
        log.Println("should not be in here")
    } else {
        log.Println("found name:", name)
    }

    time.Sleep(time.Second * 2)

    var nameAgain string
    if err := cache.Get("name", &nameAgain); err != nil {
        log.Println("not found name again")
    } else {
        log.Println("should not be in here")
    }

    cache.Set("age", 100, time.Second)
    var age int
    if err := cache.Get("age", &age); err != nil {
        log.Println("should not be in here")
    } else {
        log.Println("age setted:", age)
    }

    cache.Increment("age", 3)
    cache.Decrement("age", 1)
    var t int
    cache.Get("age", &t)
    log.Println("age>>>", t)

    log.Println(cache.Add("age", 23, time.Minute))
    cache.Replace("age", 24, time.Minute)
    var tt int
    cache.Get("age", &tt)
    log.Println("age>>>", tt)

    cache.Delete("age")
    var ageAgain int
    if err := cache.Get("age", &ageAgain); err != nil {
        log.Println("delete age successfully")
    } else {
        log.Println("should not be in here")
    }

    type User struct {
        Name string
        Age  int
    }

    cache.Set("user", &User{"Ulric", 100}, time.Minute)
    var u User
    cache.Get("user", &u)
    log.Println(u)

}
```
