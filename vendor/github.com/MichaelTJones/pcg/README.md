# pcg
Go implementation of Melissa O'Neill's excellent PCG pseudorandom number generator, which is 
well-studied, excellent, and fast both to create and in execution.

````
  Performance on a MacBook Pro:

  $ go test -v -bench=.
  === RUN   TestSanity32
  --- PASS: TestSanity32 (0.00s)
  === RUN   TestSum32
  --- PASS: TestSum32 (0.00s)
  === RUN   TestAdvance32
  --- PASS: TestAdvance32 (0.00s)
  === RUN   TestRetreat32
  --- PASS: TestRetreat32 (0.00s)
  === RUN   TestSanity64
  --- PASS: TestSanity64 (0.00s)
  === RUN   TestSum64
  --- PASS: TestSum64 (0.00s)
  === RUN   TestAdvance64
  --- PASS: TestAdvance64 (0.00s)
  === RUN   TestRetreat64
  --- PASS: TestRetreat64 (0.00s)
  === RUN   ExampleReport32
  --- PASS: ExampleReport32 (0.00s)
  === RUN   ExampleReport64
  --- PASS: ExampleReport64 (0.00s)
  BenchmarkNew32-8      2000000000               1.09 ns/op
  BenchmarkRandom32-8   1000000000               2.49 ns/op
  BenchmarkBounded32-8  200000000                9.75 ns/op
  BenchmarkNew64-8      200000000                6.89 ns/op
  BenchmarkRandom64-8   200000000                7.58 ns/op
  BenchmarkBounded64-8  50000000                25.5 ns/op
````

Provided under terms of the Apache license in keeping with Melissa O'Neill's original from which this was ported.

Copyright 2018 Michael T. Jones

   Licensed under the Apache License, Version 2.0 (the "License");
   you may not use this file except in compliance with the License.
   You may obtain a copy of the License at

       http://www.apache.org/licenses/LICENSE-2.0

   Unless required by applicable law or agreed to in writing, software
   distributed under the License is distributed on an "AS IS" BASIS,
   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
   See the License for the specific language governing permissions and
   limitations under the License.
