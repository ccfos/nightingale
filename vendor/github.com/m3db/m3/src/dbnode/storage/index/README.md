# index

Index related documentation.

## In-memory index cold flush consistency model

Index writes go into the active cold mutable segment when an index block is sealed. Index blocks are sealed during ticks when they are a full index block past buffer past + block size.

At the beginning of a cold flush we rotate out the active cold mutable segment. In-mem index writes are then evicted from mem when a cold flush completes and they are evicted.
