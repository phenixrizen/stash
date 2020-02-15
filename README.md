# Stash
Stash is a simple tiered key:value store built on top of dgraphs:
- [BadgerDB](https://github.com/dgraph-io/badger) - BadgerDB is an embeddable, persistent and fast key-value (KV) database written in pure Go.
- [Ristretto](https://github.com/dgraph-io/ristretto) - Ristretto is a fast, concurrent cache library built with a focus on performance and correctness.

Stash keeps hot records in memory and all other records are kept on disk. By default all records have a TTL.
