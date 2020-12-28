struct Data {
  1: required bool b1,
  2: required string s2,
  3: required i32 i3
}

exception SimpleErr {
  1: string message
}

exception NewErr {
  1: string message
}

service SimpleService {
  Data Call(1: Data arg)
  void Simple() throws (1: SimpleErr simpleErr)
  void SimpleFuture() throws (1: SimpleErr simpleErr, 2: NewErr newErr)
}

service SecondService {
  string Echo(1: string arg)
}

struct HealthStatus {
    1: required bool ok
    2: optional string message
}

// Meta contains the old health endpoint without arguments.
service Meta {
    HealthStatus health()
}