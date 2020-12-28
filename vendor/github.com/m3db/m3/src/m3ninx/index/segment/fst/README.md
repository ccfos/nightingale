FS Segment
===========

- Version 1.1: Adds support for a metadata proto object per Field. This is used to
store an additional postings offset per Field to a PostingsList comprising the union
of all known PostingsList across all known Terms per Field.

```
┌───────────────────────────────┐            ┌──────────────────────────────────────┐
│ FST Fields File               │            │ FST Terms File                       │
│-------------------------------│            │--------------------------------------│
│- Vellum V1 Format             │            │`n` records, each:                    │
│- []byte -> FST Terms Offset   ├─────┐      │  - metadata proto (`md-size` bytes)  │
└───────────────────────────────┘     │      │  - md-size (int64)                   │
                                      │      │  - fst payload (`fst size` bytes)    │
                                      │      │  - fst size (int64)                  │
                                      └─────▶│  - magic number (int64)              │
                                             │                                      │
                                             │Payload:                              │
                                             │(1) Vellum V1 FST                     ├─┐
                                             │[]byte -> Postings Offset             │ │
                                             │                                      │ │
                                             │(2) Metadata Proto Bytes              │ │
                                             │Field Postings Offset                 │ │
                                             └──────────────────────────────────────┘ │
                                                   ┌───────────────────────────────┐  │
                                                   │ Postings Data File            │  │
                                                   │-------------------------------│  │
                                                   │`n` records, each:             │  │
                                                   │  - payload (`size` bytes)     │  │
                                                   │  - size (int64)               │  │
                                                   │  - magic number (int64)       │◀─┘
                                                   │                               │
                                                   │Payload:                       │
                                                   │- Pilosa Bitset                ├──┐
            ┌───────────────────────────┐          │- List of doc.ID               │  │
            │ Documents Data File       │          └───────────────────────────────┘  │
            │-------------------------  │                                             │
            │'n' records, each:         │                ┌─────────────────────────┐  │
            │  - Magic Number (int64)   │                │ Documents Index File    │  │
            │  - Valid (1 byte)         │                │-------------------------│  │
            │  - Size (int64)           │                │- Magic Number (int64)   │  │
            │  - Payload (`size` bytes) │                │- Num docs (int64)       │  │
            └───────────────────────────┘        ┌───────│- Base Doc.ID `b` (int64)│◀─┘
                          ▲                      │       │- Doc `b` offset (int64) │
                          │                      │       │- Doc `b+1` offset       │
                          └──────────────────────┘       │...                      │
                                                         │- Doc `b+n-1` offset     │
                                                         └─────────────────────────┘

```


- Version 1.0: Initial Release.

```

┌───────────────────────────────┐           ┌───────────────────────────────┐
│ FST Fields File               │           │ FST Terms File                │
│-------------------------------│           │-------------------------------│
│- Vellum V1 FST                │           │`n` records, each:             │
│- []byte -> FST Terms Offset   │─────┐     │  - payload (`size` bytes)     │
└───────────────────────────────┘     │     │  - size (int64)               │
                                      └────▶│  - magic number (int64)       │
                                            │                               │
                                            │Payload:                       │
                                            │- Vellum V1 FST                │
                                            │- []byte -> Postings Offset    │
                                            └───────────────────────────────┘
        ┌───────────────────────────────┐                   │
        │ Postings Data File            │                   │
        │-------------------------------│                   │
        │`n` records, each:             │                   │
        │  - payload (`size` bytes)     │                   │
        │  - size (int64)               │                   │
        │  - magic number (int64)       │◀──────────────────┘
        │                               │
        │Payload:                       │
        │- Pilosa Bitset                │
        │- List of doc.ID               │
        └──────────┬────────────────────┘
                   │
                   │
                   │
                   │       ┌──────────────────────────┐           ┌───────────────────────────┐
                   │       │ Documents Index File     │           │ Documents Data File       │
                   │       │--------------------------│           │-------------------------  │
                   │       │- Base Doc.ID `b` (uint64)│           │'n' records, each:         │
                   │       │- Doc `b` offset (uint64) │    ┌─────▶│  - ID (bytes)             │
                   │       │- Doc `b+1` offset        │    │      │  - Fields (bytes)         │
                   └──────▶│...                       ├────┘      └───────────────────────────┘
                           │- Doc `b+n-1` offset      │
                           └──────────────────────────┘
```