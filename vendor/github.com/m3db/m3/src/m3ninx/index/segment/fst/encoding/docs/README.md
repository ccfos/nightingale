# Documents

Two files are used to represent the documents in a segment. The data file contains the
data for each document in the segment. The index file contains, for each document, its
corresponding offset in the data file.

## Data File

The data file contains the fields for each document. The documents are stored serially.

```
┌───────────────────────────┐
│ ┌───────────────────────┐ │
│ │      Document 1       │ │
│ ├───────────────────────┤ │
│ │          ...          │ │
│ ├───────────────────────┤ │
│ │      Document n       │ │
│ └───────────────────────┘ │
└───────────────────────────┘
```

### Document

Each document is composed of an ID and its fields. The ID is a sequence of valid UTF-8 bytes
and it is encoded first by encoding the length of the ID, in bytes, as a variable-sized
unsigned integer and then encoding the actual bytes which comprise the ID. Following the ID
are the fields. The number of fields in the document is encoded first as a variable-sized
unsigned integer and then the fields themselves are encoded.

```
┌───────────────────────────┐
│ ┌───────────────────────┐ │
│ │     Length of ID      │ │
│ │       (uvarint)       │ │
│ ├───────────────────────┤ │
│ │                       │ │
│ │          ID           │ │
│ │        (bytes)        │ │
│ │                       │ │
│ ├───────────────────────┤ │
│ │   Number of Fields    │ │
│ │       (uvarint)       │ │
│ ├───────────────────────┤ │
│ │                       │ │
│ │        Field 1        │ │
│ │                       │ │
│ ├───────────────────────┤ │
│ │                       │ │
│ │          ...          │ │
│ │                       │ │
│ ├───────────────────────┤ │
│ │                       │ │
│ │        Field n        │ │
│ │                       │ │
│ └───────────────────────┘ │
└───────────────────────────┘
```

#### Field

Each field is composed of a name and a value. The name and value are a sequence of valid
UTF-8 bytes and they are stored by encoding the length of the name (value), in bytes, as a
variable-sized unsigned integer and then encoding the actual bytes which comprise the name
(value). The name is encoded first and the value second.

```
┌───────────────────────────┐
│ ┌───────────────────────┐ │
│ │  Length of Field Name │ │
│ │       (uvarint)       │ │
│ ├───────────────────────┤ │
│ │                       │ │
│ │      Field Name       │ │
│ │        (bytes)        │ │
│ │                       │ │
│ ├───────────────────────┤ │
│ │ Length of Field Value │ │
│ │       (uvarint)       │ │
│ ├───────────────────────┤ │
│ │                       │ │
│ │      Field Value      │ │
│ │        (bytes)        │ │
│ │                       │ │
│ └───────────────────────┘ │
└───────────────────────────┘
```

## Index File

The index file contains, for each postings ID in the segment, the offset of the corresponding
document in the data file. The base postings ID is stored at the start of the file as a
little-endian `uint64`. Following it are the actual offsets.

```
┌───────────────────────────┐
│            Base           │
│          (uint64)         │
├───────────────────────────┤
│                           │
│                           │
│          Offsets          │
│                           │
│                           │
└───────────────────────────┘
```

### Offsets

The offsets are stored serially starting from the offset for the base postings ID. Each
offset is a little-endian `uint64`. Since each offset is of a fixed-size we can access
the offset for a given postings ID by calculating its index relative to the start of
the offsets. An offset equal to the maximum value for a uint64 indicates that there is
no corresponding document for a given postings ID.

```
┌───────────────────────────┐
│ ┌───────────────────────┐ │
│ │       Offset 1        │ │
│ │       (uint64)        │ │
│ ├───────────────────────┤ │
│ │          ...          │ │
│ ├───────────────────────┤ │
│ │       Offset n        │ │
│ │       (uint64)        │ │
│ └───────────────────────┘ │
└───────────────────────────┘
```
