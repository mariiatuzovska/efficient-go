# Data-Driven Performance Optimization for Reading Big Files

This project demonstrates a practical, *data-driven* approach to processing massive log files in Go, inspired by the principles from my favorite book **“Efficient Go”**.

The goal: **count unique IPv4 addresses in multi-GB files** with predictable memory usage, high throughput, graceful cancellation, and no hidden allocations.

---

## Why This Exists

Modern systems generate huge volumes of logs. Grepping gigabytes is slow. Loading everything into memory is wasteful. Hash maps thrash caches.

This project explores **how to do it right**:

- Measure first, optimize later.
- Prefer predictable memory over dynamic growth.
- Minimize allocations and branches in the hot loop.
- Exploit I/O and CPU parallelism without locking.
- Stop gracefully with `Ctrl+C` and still produce a correct result.

---

## Architecture Overview

| Component | Purpose |
|----------|--------|
| `mmap + overlapped chunks` | Avoids copying data; avoids splitting lines across chunks |
| Worker goroutines | Parallel parsing and IP extraction |
| Lock-free bitmap | 512MB bit-set → 1 bit per IPv4 address |
| Manual parser | Avoids allocations and `strings.Split` |
| `signal.NotifyContext` | Graceful cancel with final result printed |
| Data-driven tuning | Chunk size, overlap, worker count configurable |

### Memory Model

| Resource | Usage |
|--------|------|
| Bitmap | `2^32 / 8` bytes = **512MB** |
| Workers | Only `chunk + overlap` resident each |
| goroutines | CPU-bound parsing; no cross-worker locks |

---

## Running

### Build

```sh
go build -o uniqip
