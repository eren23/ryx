# Swarm Goal — Ryx Standard Library Expansion + Map<K,V>

Add a built-in `Map` collection type and expand the standard library with
math, time, random, array, string, and filesystem functions.  **No parser,
resolver, type-checker, HIR, MIR, or codegen changes are required.**
Everything is implemented through the existing `BuiltinRegistry` mechanism
and heap object system.

---

## 0) Overview & Motivation

The Ryx language is feature-complete in its compiler pipeline but its
standard library has significant gaps and it lacks the most fundamental
collection type: a hash map / dictionary.

The existing `tests/testdata/programs/hash_map.ryx` simulates a hash map
with parallel arrays.  After this project, that program can be rewritten
with real map operations:

### Sample Program: Word Counter
```ryx
fn main() {
    let words = string_split("the cat sat on the mat the cat", " ");
    let mut counts = map_new();
    let mut i = 0;
    while i < array_len(words) {
        let w = words[i];
        if map_contains(counts, w) {
            let c = map_get(counts, w);
            counts = map_set(counts, w, c + 1);
        } else {
            counts = map_set(counts, w, 1);
        };
        i = i + 1;
    };
    println(map_len(counts));
    println(map_get(counts, "the"));
    println(map_get(counts, "cat"));
    println(map_get(counts, "sat"));
    println(map_contains(counts, "dog"));
}
```
Expected output:
```
5
3
2
1
false
```

### Sample Program: Math & Conversions
```ryx
fn main() {
    println(sin(pi() / 2.0));
    println(cos(0.0));
    println(floor(3.7));
    println(ceil(3.2));
    println(gcd(12, 8));
    println(clamp(15, 0, 10));
    println(bool_to_string(true));
    println(int_to_char(65));
}
```
Expected output:
```
1
1
3
4
4
10
true
A
```

### Sample Program: Array & String Utilities
```ryx
fn main() {
    let nums = [3, 1, 4, 1, 5, 9, 2, 6];
    println(array_sum(nums));
    println(array_min(nums));
    println(array_max(nums));
    println(array_any(nums, |x: Int| -> Bool { x > 8 }));
    println(array_all(nums, |x: Int| -> Bool { x > 0 }));

    let uniq = array_unique(nums);
    println(array_len(uniq));

    let chunks = array_chunk([1,2,3,4,5], 2);
    println(array_len(chunks));

    println(string_repeat("ab", 3));
    println(string_pad_left("42", 5, '0'));
    println(string_index_of("hello world", "world"));
    println(string_join(["a", "b", "c"], "-"));
}
```
Expected output:
```
31
1
9
true
true
7
3
ababab
00042
6
a-b-c
```

### What Success Looks Like

After this project completes:
- `go test ./pkg/vm/... -v` passes with new Map heap type tests
- `go test ./pkg/gc/... -v` passes with Map GC integration tests
- `go test ./pkg/stdlib/... -v` passes with all new stdlib function tests
- `go test ./tests/integration/... -v` passes with new `.ryx` test programs
- All 753+ existing tests continue to pass

---

## 1) Map<K,V> Heap Type

### Architectural Insight

Map does NOT need any compiler changes.  The existing `BuiltinRegistry`
mechanism supports calling `map_new()`, `map_get(m, k)`, `map_set(m, k, v)`
as plain function calls — exactly how array operations work.  Map is
implemented as a **heap-resident object** (like `ArrayObj`, `StructObj`)
with **stdlib functions** (like `array_ops.go`).

### 1.1) ObjMap Constant

In `pkg/vm/value.go`, add the new object type constant:

```go
const (
    ObjString  byte = 0x20
    ObjArray   byte = 0x21
    ObjTuple   byte = 0x22
    ObjStruct  byte = 0x23
    ObjEnum    byte = 0x24
    ObjClosure byte = 0x25
    ObjChannel byte = 0x26
    ObjMap     byte = 0x27  // ← NEW
)
```

### 1.2) MapObj Struct

In `pkg/vm/value.go`, add these types:

```go
// mapEntry represents one key-value pair in a hash bucket.
// Collisions within the same bucket are handled by linear scan
// using Value.Equal() for key comparison.
type mapEntry struct {
    Key   Value
    Value Value
}

// MapObj is a hash map from any hashable Value to any Value.
// Uses FNV-1a hashing (via the existing hashValue function) and
// separate chaining with Go slices for collision resolution.
type MapObj struct {
    Buckets    map[uint64][]mapEntry  // hash → chain of entries
    Count      int                    // number of entries (O(1) len)
}
```

**Design rationale:**
- `map[uint64][]mapEntry` — Go map keyed by 64-bit FNV-1a hash of
  the Ryx `Value`.  Collisions within the same hash are resolved by
  linear scan of the `[]mapEntry` slice, comparing keys with
  `Value.Equal()`.
- This reuses `BuiltinHash` / `hashValue` which already handles all
  Ryx value types (Int, Float, Bool, Char, String, Array, Tuple,
  Struct, Enum).
- `Count` avoids O(n) length computation.

**Key types that are hashable** (already supported by `hashValue`):
- Int, Float, Bool, Char, Unit (primitives)
- String, Array, Tuple, Struct, Enum (heap objects)

**Key types that are NOT hashable** (no `hashValue` case):
- Closure, Channel — using these as keys is a runtime error

### 1.3) Helper: computeHash

Add a helper function accessible to both `value.go` and `map_ops.go`:

```go
// computeHash returns the FNV-1a hash of a Value.
// Used internally by map operations. Errors on unhashable types
// (Closure, Channel).
func ComputeHash(v Value, heap *Heap) (uint64, error) {
    if v.Tag == TagObj {
        obj := heap.Get(v.AsObj())
        switch obj.Data.(type) {
        case *ClosureObj:
            return 0, fmt.Errorf("unhashable type: closure")
        case *ChannelObj:
            return 0, fmt.Errorf("unhashable type: channel")
        }
    }
    h := fnv.New64a()
    hashValue(v, heap, h)
    return h.Sum64(), nil
}
```

### 1.4) MapObj Methods

Add these methods on `MapObj` (in `pkg/vm/value.go` or a new
`pkg/vm/map.go` file — either is acceptable):

```go
// Get retrieves the value for key. Returns (value, true) if found,
// (UnitVal(), false) if not found.
func (m *MapObj) Get(key Value, heap *Heap) (Value, bool, error) {
    hash, err := ComputeHash(key, heap)
    if err != nil {
        return UnitVal(), false, err
    }
    bucket := m.Buckets[hash]
    for _, entry := range bucket {
        if entry.Key.Equal(key, heap) {
            return entry.Value, true, nil
        }
    }
    return UnitVal(), false, nil
}

// Set inserts or updates a key-value pair. Returns true if the key
// was newly inserted, false if it was updated.
func (m *MapObj) Set(key, value Value, heap *Heap) (bool, error) {
    hash, err := ComputeHash(key, heap)
    if err != nil {
        return false, err
    }
    bucket := m.Buckets[hash]
    for i, entry := range bucket {
        if entry.Key.Equal(key, heap) {
            m.Buckets[hash][i].Value = value
            return false, nil  // updated existing
        }
    }
    m.Buckets[hash] = append(bucket, mapEntry{Key: key, Value: value})
    m.Count++
    return true, nil  // newly inserted
}

// Delete removes a key. Returns true if the key existed.
func (m *MapObj) Delete(key Value, heap *Heap) (bool, error) {
    hash, err := ComputeHash(key, heap)
    if err != nil {
        return false, err
    }
    bucket := m.Buckets[hash]
    for i, entry := range bucket {
        if entry.Key.Equal(key, heap) {
            // Remove by swapping with last element
            m.Buckets[hash][i] = m.Buckets[hash][len(bucket)-1]
            m.Buckets[hash] = m.Buckets[hash][:len(bucket)-1]
            if len(m.Buckets[hash]) == 0 {
                delete(m.Buckets, hash)
            }
            m.Count--
            return true, nil
        }
    }
    return false, nil
}

// Contains checks if a key exists. O(1) average case.
func (m *MapObj) Contains(key Value, heap *Heap) (bool, error) {
    _, found, err := m.Get(key, heap)
    return found, err
}

// Keys returns all keys as a slice of Values.
// Order is non-deterministic (Go map iteration order).
func (m *MapObj) Keys() []Value {
    keys := make([]Value, 0, m.Count)
    for _, bucket := range m.Buckets {
        for _, entry := range bucket {
            keys = append(keys, entry.Key)
        }
    }
    return keys
}

// Values returns all values as a slice of Values.
func (m *MapObj) Values() []Value {
    vals := make([]Value, 0, m.Count)
    for _, bucket := range m.Buckets {
        for _, entry := range bucket {
            vals = append(vals, entry.Value)
        }
    }
    return vals
}

// Entries returns all entries as a slice of (key, value) pairs.
func (m *MapObj) Entries() []mapEntry {
    entries := make([]mapEntry, 0, m.Count)
    for _, bucket := range m.Buckets {
        entries = append(entries, bucket...)
    }
    return entries
}
```

### 1.5) AllocMap on Heap

In `pkg/vm/heap.go`, add:

```go
func (h *Heap) AllocMap() uint32 {
    return h.Alloc(HeapObject{
        Header: ObjectHeader{TypeID: ObjMap, Size: 0},
        Data: &MapObj{
            Buckets: make(map[uint64][]mapEntry),
            Count:   0,
        },
    })
}
```

Note: `Size` in the header is updated to `m.Count` when accessed
via `StringValue` or for display, but the canonical count is
`MapObj.Count`.

### 1.6) Value Trait Implementations for MapObj

Each of these functions in `pkg/vm/builtins.go` (or `value.go` as
appropriate) needs a new `case *MapObj` branch:

#### Equal()
```go
case *MapObj:
    sb := ob.Data.(*MapObj)
    if sa.Count != sb.Count {
        return false
    }
    // For each entry in sa, check sb has same key→value
    for _, bucket := range sa.Buckets {
        for _, entry := range bucket {
            val, found, _ := sb.Get(entry.Key, heap)
            if !found || !entry.Value.Equal(val, heap) {
                return false
            }
        }
    }
    return true
```

#### StringValue()
```go
case *MapObj:
    if o.Count == 0 {
        return "{}"
    }
    parts := make([]string, 0, o.Count)
    for _, bucket := range o.Buckets {
        for _, entry := range bucket {
            k := StringValue(entry.Key, heap)
            v := StringValue(entry.Value, heap)
            parts = append(parts, k+": "+v)
        }
    }
    return "{" + strings.Join(parts, ", ") + "}"
```

#### deepClone()
```go
case *MapObj:
    newBuckets := make(map[uint64][]mapEntry, len(o.Buckets))
    for hash, bucket := range o.Buckets {
        newBucket := make([]mapEntry, len(bucket))
        for i, entry := range bucket {
            newBucket[i] = mapEntry{
                Key:   deepClone(entry.Key, heap),
                Value: deepClone(entry.Value, heap),
            }
        }
        newBuckets[hash] = newBucket
    }
    idx := heap.Alloc(HeapObject{
        Header: ObjectHeader{TypeID: ObjMap, Size: uint32(o.Count)},
        Data: &MapObj{
            Buckets: newBuckets,
            Count:   o.Count,
        },
    })
    return ObjVal(idx)
```

#### hashValue()
```go
case *MapObj:
    // Hash maps are order-independent. To produce a consistent hash
    // regardless of insertion order, XOR the hashes of all entries.
    var combined uint64
    for _, bucket := range o.Buckets {
        for _, entry := range bucket {
            eh := fnv.New64a()
            hashValue(entry.Key, heap, eh)
            hashValue(entry.Value, heap, eh)
            combined ^= eh.Sum64()
        }
    }
    b := [8]byte{}
    for i := 0; i < 8; i++ {
        b[i] = byte(combined >> (i * 8))
    }
    h.Write(b[:])
```

#### deepCompare()
```go
case *MapObj:
    sb := ob.Data.(*MapObj)
    // Compare by count first
    if sa.Count < sb.Count {
        return -1
    }
    if sa.Count > sb.Count {
        return 1
    }
    return 0  // Maps with same count are equal in ordering
```

---

## 2) GC Integration for Maps

### 2.1) KindMap Object Kind

In `pkg/gc/gc.go`, add `KindMap` to the `ObjectKind` enum:

```go
type ObjectKind byte
const (
    KindArray   ObjectKind = iota
    KindStruct
    KindEnum
    KindClosure
    KindChannel
    KindString
    KindUpvalue
    KindFiber
    KindMap     // ← NEW
)
```

### 2.2) GC Object for Map

When a `MapObj` is allocated on the VM heap, a corresponding GC
`Object` must be created with `KindMap` and all referenced Values
traced as children.

**Children of a MapObj:**
- Every key that is a heap object (`Tag == TagObj`)
- Every value that is a heap object (`Tag == TagObj`)

Both keys AND values must be traced.  If a key references a String
and a value references an Array, both the String and Array must
survive GC as long as the Map is reachable.

### 2.3) Tracing Pattern

Follow the channel GC integration pattern.  In the VM's GC bridge
(where `HeapObject` → `gc.Object` conversion happens), add:

```go
case *MapObj:
    var children []*gc.Object
    for _, bucket := range m.Buckets {
        for _, entry := range bucket {
            if entry.Key.Tag == vm.TagObj {
                children = append(children, gcObjectFor(entry.Key.AsObj()))
            }
            if entry.Value.Tag == vm.TagObj {
                children = append(children, gcObjectFor(entry.Value.AsObj()))
            }
        }
    }
    gcObj := heap.Alloc(gc.KindMap, estimateMapSize(m), children...)
```

### 2.4) Write Barrier Triggers

Write barriers must fire on mutation operations:

- **`map_set(m, k, v)`** — when inserting or updating an entry, both
  the key and value may be new heap references.  Call
  `BarrierMapSet(mapGCObj, keyGCObj, valueGCObj)`.

- **`map_delete(m, k)`** — removal does not create new references,
  so no barrier is needed.  However, if the GC uses a snapshot-at-
  the-beginning strategy, the barrier should also fire on delete to
  re-gray the map.  Follow the same strategy as `BarrierIndexSet`.

- **`map_merge(m1, m2)`** — creates a new map, so all children of
  the new map are set at allocation time.  No barrier needed for
  the new map (it starts Gray during mark phase per `Alloc` rules).

Add convenience wrapper in `pkg/gc/write_barrier.go`:

```go
func (h *Heap) BarrierMapSet(mapObj, keyObj, valueObj *Object) {
    h.WriteBarrier(mapObj, keyObj)
    h.WriteBarrier(mapObj, valueObj)
}
```

### 2.5) Size Estimation

For GC accounting, estimate MapObj size as:

```go
func estimateMapSize(m *MapObj) uint64 {
    // Base overhead: Go map header + bucket pointers
    // Plus entry count × (key Value 16 bytes + value Value 16 bytes)
    return uint64(64 + m.Count*32)
}
```

---

## 3) Map Stdlib Functions (12 functions)

All functions go in a new file `pkg/stdlib/map_ops.go` and are
registered in `pkg/stdlib/core.go` via `RegisterAll`.

### Registration

Add to `RegisterAll` in `core.go`:

```go
// Map operations (map_ops.go)
r.Register("map_new", MapNew)
r.Register("map_get", MapGet)
r.Register("map_set", MapSet)
r.Register("map_delete", MapDelete)
r.Register("map_contains", MapContains)
r.Register("map_len", MapLen)
r.Register("map_keys", MapKeys)
r.Register("map_values", MapValues)
r.Register("map_entries", MapEntries)
r.Register("map_merge", MapMerge)
r.Register("map_filter", MapFilter)
r.Register("map_map", MapMapValues)
```

### Helper: resolveMap

```go
func resolveMap(v vm.Value, heap *vm.Heap) (*vm.MapObj, error) {
    if v.Tag != vm.TagObj {
        return nil, fmt.Errorf("expected map, got tag=%d", v.Tag)
    }
    obj := heap.Get(v.AsObj())
    if obj.Header.TypeID != vm.ObjMap {
        return nil, fmt.Errorf("expected map, got object type=%d", obj.Header.TypeID)
    }
    m, ok := obj.Data.(*vm.MapObj)
    if !ok {
        return nil, fmt.Errorf("expected map object data")
    }
    return m, nil
}
```

### 3.1) map_new

```
Signature:  map_new() -> Map
Arguments:  0
Returns:    A new empty map (heap-allocated MapObj)
Errors:     None
```

```go
func MapNew(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
    if len(args) != 0 {
        return vm.UnitVal(), fmt.Errorf("map_new: expected 0 args, got %d", len(args))
    }
    idx := heap.AllocMap()
    return vm.ObjVal(idx), nil
}
```

### 3.2) map_get

```
Signature:  map_get(m: Map, key: Any) -> Any
Arguments:  2 (map, key)
Returns:    The value associated with key
Errors:     Runtime error if key not found (NOT a Result — keeps API simple)
            Runtime error if key type is unhashable (Closure, Channel)
```

```go
func MapGet(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
    if len(args) != 2 {
        return vm.UnitVal(), fmt.Errorf("map_get: expected 2 args, got %d", len(args))
    }
    m, err := resolveMap(args[0], heap)
    if err != nil {
        return vm.UnitVal(), fmt.Errorf("map_get: %w", err)
    }
    val, found, err := m.Get(args[1], heap)
    if err != nil {
        return vm.UnitVal(), fmt.Errorf("map_get: %w", err)
    }
    if !found {
        keyStr := vm.StringValue(args[1], heap)
        return vm.UnitVal(), fmt.Errorf("map_get: key not found: %s", keyStr)
    }
    return val, nil
}
```

**Edge cases:**
- `map_get(empty_map, "x")` → runtime error "key not found: x"
- `map_get(m, closure_val)` → runtime error "unhashable type: closure"

### 3.3) map_set

```
Signature:  map_set(m: Map, key: Any, value: Any) -> Map
Arguments:  3 (map, key, value)
Returns:    The same map (mutated in place) for chaining convenience
Errors:     Runtime error if key type is unhashable
```

**Important:** `map_set` mutates the map in place (like `array_push`
mutates the array).  It returns the same map object for ergonomic
chaining: `m = map_set(m, k, v)`.

```go
func MapSet(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
    if len(args) != 3 {
        return vm.UnitVal(), fmt.Errorf("map_set: expected 3 args, got %d", len(args))
    }
    m, err := resolveMap(args[0], heap)
    if err != nil {
        return vm.UnitVal(), fmt.Errorf("map_set: %w", err)
    }
    _, err = m.Set(args[1], args[2], heap)
    if err != nil {
        return vm.UnitVal(), fmt.Errorf("map_set: %w", err)
    }
    return args[0], nil  // return same map for chaining
}
```

**Edge cases:**
- `map_set(m, "key", 42)` when "key" already exists → updates value, returns m
- `map_set(m, "key", 42)` when "key" doesn't exist → inserts, returns m
- `map_set(m, closure, 1)` → runtime error

### 3.4) map_delete

```
Signature:  map_delete(m: Map, key: Any) -> Map
Arguments:  2 (map, key)
Returns:    The same map (mutated) with the key removed
Errors:     Runtime error if key type is unhashable
            Silently succeeds if key not found (idempotent)
```

```go
func MapDelete(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
    if len(args) != 2 {
        return vm.UnitVal(), fmt.Errorf("map_delete: expected 2 args, got %d", len(args))
    }
    m, err := resolveMap(args[0], heap)
    if err != nil {
        return vm.UnitVal(), fmt.Errorf("map_delete: %w", err)
    }
    _, err = m.Delete(args[1], heap)
    if err != nil {
        return vm.UnitVal(), fmt.Errorf("map_delete: %w", err)
    }
    return args[0], nil
}
```

### 3.5) map_contains

```
Signature:  map_contains(m: Map, key: Any) -> Bool
Arguments:  2 (map, key)
Returns:    true if key exists, false otherwise
Errors:     Runtime error if key type is unhashable
```

```go
func MapContains(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
    if len(args) != 2 {
        return vm.UnitVal(), fmt.Errorf("map_contains: expected 2 args, got %d", len(args))
    }
    m, err := resolveMap(args[0], heap)
    if err != nil {
        return vm.UnitVal(), fmt.Errorf("map_contains: %w", err)
    }
    found, err := m.Contains(args[1], heap)
    if err != nil {
        return vm.UnitVal(), fmt.Errorf("map_contains: %w", err)
    }
    return vm.BoolVal(found), nil
}
```

### 3.6) map_len

```
Signature:  map_len(m: Map) -> Int
Arguments:  1 (map)
Returns:    Number of key-value pairs
Errors:     None (argument type checked)
```

```go
func MapLen(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
    if len(args) != 1 {
        return vm.UnitVal(), fmt.Errorf("map_len: expected 1 arg, got %d", len(args))
    }
    m, err := resolveMap(args[0], heap)
    if err != nil {
        return vm.UnitVal(), fmt.Errorf("map_len: %w", err)
    }
    return vm.IntVal(int64(m.Count)), nil
}
```

### 3.7) map_keys

```
Signature:  map_keys(m: Map) -> [Any]
Arguments:  1 (map)
Returns:    Array of all keys (order is non-deterministic)
Errors:     None
```

```go
func MapKeys(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
    if len(args) != 1 {
        return vm.UnitVal(), fmt.Errorf("map_keys: expected 1 arg, got %d", len(args))
    }
    m, err := resolveMap(args[0], heap)
    if err != nil {
        return vm.UnitVal(), fmt.Errorf("map_keys: %w", err)
    }
    keys := m.Keys()
    idx := heap.AllocArray(keys)
    return vm.ObjVal(idx), nil
}
```

**Note on ordering:** Since Go maps have non-deterministic iteration
order, `map_keys` returns keys in an unspecified order.  Test programs
should test key presence (via `array_contains`) or sort keys before
comparing, NOT rely on a specific order.

### 3.8) map_values

```
Signature:  map_values(m: Map) -> [Any]
Arguments:  1 (map)
Returns:    Array of all values (order matches map_keys)
Errors:     None
```

```go
func MapValues(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
    if len(args) != 1 {
        return vm.UnitVal(), fmt.Errorf("map_values: expected 1 arg, got %d", len(args))
    }
    m, err := resolveMap(args[0], heap)
    if err != nil {
        return vm.UnitVal(), fmt.Errorf("map_values: %w", err)
    }
    vals := m.Values()
    idx := heap.AllocArray(vals)
    return vm.ObjVal(idx), nil
}
```

### 3.9) map_entries

```
Signature:  map_entries(m: Map) -> [(Any, Any)]
Arguments:  1 (map)
Returns:    Array of (key, value) tuples
Errors:     None
```

```go
func MapEntries(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
    if len(args) != 1 {
        return vm.UnitVal(), fmt.Errorf("map_entries: expected 1 arg, got %d", len(args))
    }
    m, err := resolveMap(args[0], heap)
    if err != nil {
        return vm.UnitVal(), fmt.Errorf("map_entries: %w", err)
    }
    entries := m.Entries()
    tuples := make([]vm.Value, len(entries))
    for i, e := range entries {
        idx := heap.AllocTuple([]vm.Value{e.Key, e.Value})
        tuples[i] = vm.ObjVal(idx)
    }
    arrIdx := heap.AllocArray(tuples)
    return vm.ObjVal(arrIdx), nil
}
```

### 3.10) map_merge

```
Signature:  map_merge(m1: Map, m2: Map) -> Map
Arguments:  2 (map, map)
Returns:    A NEW map containing all entries from both maps.
            If both maps have the same key, m2's value wins.
Errors:     None (both args type-checked)
```

```go
func MapMerge(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
    if len(args) != 2 {
        return vm.UnitVal(), fmt.Errorf("map_merge: expected 2 args, got %d", len(args))
    }
    m1, err := resolveMap(args[0], heap)
    if err != nil {
        return vm.UnitVal(), fmt.Errorf("map_merge: arg 1: %w", err)
    }
    m2, err := resolveMap(args[1], heap)
    if err != nil {
        return vm.UnitVal(), fmt.Errorf("map_merge: arg 2: %w", err)
    }

    // Create new map, copy m1 entries, then m2 entries (m2 wins on conflict)
    newIdx := heap.AllocMap()
    newMap := heap.Get(newIdx).Data.(*vm.MapObj)

    for _, bucket := range m1.Buckets {
        for _, entry := range bucket {
            newMap.Set(entry.Key, entry.Value, heap)
        }
    }
    for _, bucket := range m2.Buckets {
        for _, entry := range bucket {
            newMap.Set(entry.Key, entry.Value, heap)
        }
    }

    return vm.ObjVal(newIdx), nil
}
```

### 3.11) map_filter

```
Signature:  map_filter(m: Map, predicate: (Any, Any) -> Bool) -> Map
Arguments:  2 (map, function taking key and value, returning Bool)
Returns:    A NEW map containing only entries where predicate returns true
Errors:     Runtime error if CallbackInvoker is nil
            Runtime error if predicate errors
```

Uses the `CallbackInvoker` pattern from `array_ops.go`:

```go
func MapFilter(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
    if len(args) != 2 {
        return vm.UnitVal(), fmt.Errorf("map_filter: expected 2 args, got %d", len(args))
    }
    m, err := resolveMap(args[0], heap)
    if err != nil {
        return vm.UnitVal(), fmt.Errorf("map_filter: %w", err)
    }
    fn := args[1]
    if CallbackInvoker == nil {
        return vm.UnitVal(), fmt.Errorf("map_filter: no callback invoker registered")
    }

    newIdx := heap.AllocMap()
    newMap := heap.Get(newIdx).Data.(*vm.MapObj)

    for _, bucket := range m.Buckets {
        for _, entry := range bucket {
            result, callErr := CallbackInvoker(fn, []vm.Value{entry.Key, entry.Value}, heap)
            if callErr != nil {
                return vm.UnitVal(), fmt.Errorf("map_filter: callback error: %w", callErr)
            }
            if result.IsTruthy() {
                newMap.Set(entry.Key, entry.Value, heap)
            }
        }
    }

    return vm.ObjVal(newIdx), nil
}
```

### 3.12) map_map

```
Signature:  map_map(m: Map, transform: (Any, Any) -> Any) -> Map
Arguments:  2 (map, function taking key and value, returning new value)
Returns:    A NEW map with same keys but transformed values
Errors:     Runtime error if CallbackInvoker is nil
            Runtime error if transform errors
```

```go
func MapMapValues(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
    if len(args) != 2 {
        return vm.UnitVal(), fmt.Errorf("map_map: expected 2 args, got %d", len(args))
    }
    m, err := resolveMap(args[0], heap)
    if err != nil {
        return vm.UnitVal(), fmt.Errorf("map_map: %w", err)
    }
    fn := args[1]
    if CallbackInvoker == nil {
        return vm.UnitVal(), fmt.Errorf("map_map: no callback invoker registered")
    }

    newIdx := heap.AllocMap()
    newMap := heap.Get(newIdx).Data.(*vm.MapObj)

    for _, bucket := range m.Buckets {
        for _, entry := range bucket {
            result, callErr := CallbackInvoker(fn, []vm.Value{entry.Key, entry.Value}, heap)
            if callErr != nil {
                return vm.UnitVal(), fmt.Errorf("map_map: callback error: %w", callErr)
            }
            newMap.Set(entry.Key, result, heap)
        }
    }

    return vm.ObjVal(newIdx), nil
}
```

---

## 4) Math Stdlib Expansion (16 functions)

All functions go in `pkg/stdlib/math_ops.go`.  All take Int or Float
arguments (auto-promoted to Float where needed via the existing
`toFloat` helper) and return Float unless otherwise noted.

### Registration

Add to `RegisterAll` in `core.go`:

```go
// Trigonometric (math_ops.go)
r.Register("sin", Sin)
r.Register("cos", Cos)
r.Register("tan", Tan)
r.Register("asin", Asin)
r.Register("acos", Acos)
r.Register("atan", Atan)
r.Register("atan2", Atan2)

// Logarithmic/Exponential (math_ops.go)
r.Register("log", Log)
r.Register("log2", Log2)
r.Register("log10", Log10)
r.Register("exp", Exp)

// Constants (math_ops.go)
r.Register("pi", Pi)
r.Register("e", E)

// Integer math (math_ops.go)
r.Register("gcd", Gcd)
r.Register("lcm", Lcm)
r.Register("clamp", Clamp)
```

### 4.1) sin

```
Signature:  sin(x: Float) -> Float
Arguments:  1 (Float or Int, auto-promoted)
Returns:    Sine of x (x in radians)
Edge cases: sin(0.0) = 0.0, sin(pi/2) = 1.0, sin(NaN) = NaN, sin(±Inf) = NaN
```

```go
func Sin(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
    if len(args) != 1 {
        return vm.UnitVal(), fmt.Errorf("sin: expected 1 arg, got %d", len(args))
    }
    return vm.FloatVal(math.Sin(toFloat(args[0]))), nil
}
```

### 4.2) cos

```
Signature:  cos(x: Float) -> Float
Arguments:  1
Returns:    Cosine of x (x in radians)
Edge cases: cos(0.0) = 1.0, cos(pi) = -1.0, cos(NaN) = NaN, cos(±Inf) = NaN
```

```go
func Cos(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
    if len(args) != 1 {
        return vm.UnitVal(), fmt.Errorf("cos: expected 1 arg, got %d", len(args))
    }
    return vm.FloatVal(math.Cos(toFloat(args[0]))), nil
}
```

### 4.3) tan

```
Signature:  tan(x: Float) -> Float
Arguments:  1
Returns:    Tangent of x
Edge cases: tan(0.0) = 0.0, tan(pi/2) ≈ very large, tan(NaN) = NaN
```

```go
func Tan(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
    if len(args) != 1 {
        return vm.UnitVal(), fmt.Errorf("tan: expected 1 arg, got %d", len(args))
    }
    return vm.FloatVal(math.Tan(toFloat(args[0]))), nil
}
```

### 4.4) asin

```
Signature:  asin(x: Float) -> Float
Arguments:  1
Returns:    Arcsine of x (result in radians, range [-pi/2, pi/2])
Edge cases: asin(0.0) = 0.0, asin(1.0) = pi/2
            asin(x) where |x| > 1 = NaN (domain error)
```

```go
func Asin(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
    if len(args) != 1 {
        return vm.UnitVal(), fmt.Errorf("asin: expected 1 arg, got %d", len(args))
    }
    return vm.FloatVal(math.Asin(toFloat(args[0]))), nil
}
```

### 4.5) acos

```
Signature:  acos(x: Float) -> Float
Arguments:  1
Returns:    Arccosine of x (result in radians, range [0, pi])
Edge cases: acos(1.0) = 0.0, acos(0.0) = pi/2
            acos(x) where |x| > 1 = NaN (domain error)
```

```go
func Acos(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
    if len(args) != 1 {
        return vm.UnitVal(), fmt.Errorf("acos: expected 1 arg, got %d", len(args))
    }
    return vm.FloatVal(math.Acos(toFloat(args[0]))), nil
}
```

### 4.6) atan

```
Signature:  atan(x: Float) -> Float
Arguments:  1
Returns:    Arctangent of x (result in radians, range (-pi/2, pi/2))
Edge cases: atan(0.0) = 0.0, atan(1.0) = pi/4, atan(±Inf) = ±pi/2
```

```go
func Atan(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
    if len(args) != 1 {
        return vm.UnitVal(), fmt.Errorf("atan: expected 1 arg, got %d", len(args))
    }
    return vm.FloatVal(math.Atan(toFloat(args[0]))), nil
}
```

### 4.7) atan2

```
Signature:  atan2(y: Float, x: Float) -> Float
Arguments:  2
Returns:    Angle in radians between positive x-axis and point (x, y)
            Range (-pi, pi]
Edge cases: atan2(0, 1) = 0, atan2(1, 0) = pi/2, atan2(0, -1) = pi
```

```go
func Atan2(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
    if len(args) != 2 {
        return vm.UnitVal(), fmt.Errorf("atan2: expected 2 args, got %d", len(args))
    }
    return vm.FloatVal(math.Atan2(toFloat(args[0]), toFloat(args[1]))), nil
}
```

### 4.8) log

```
Signature:  log(x: Float) -> Float
Arguments:  1
Returns:    Natural logarithm (base e) of x
Edge cases: log(1.0) = 0.0, log(e()) ≈ 1.0
            log(0.0) = -Inf, log(-1.0) = NaN
```

```go
func Log(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
    if len(args) != 1 {
        return vm.UnitVal(), fmt.Errorf("log: expected 1 arg, got %d", len(args))
    }
    return vm.FloatVal(math.Log(toFloat(args[0]))), nil
}
```

### 4.9) log2

```
Signature:  log2(x: Float) -> Float
Arguments:  1
Returns:    Base-2 logarithm of x
Edge cases: log2(1.0) = 0.0, log2(8.0) = 3.0
            log2(0.0) = -Inf, log2(-1.0) = NaN
```

```go
func Log2(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
    if len(args) != 1 {
        return vm.UnitVal(), fmt.Errorf("log2: expected 1 arg, got %d", len(args))
    }
    return vm.FloatVal(math.Log2(toFloat(args[0]))), nil
}
```

### 4.10) log10

```
Signature:  log10(x: Float) -> Float
Arguments:  1
Returns:    Base-10 logarithm of x
Edge cases: log10(1.0) = 0.0, log10(100.0) = 2.0
            log10(0.0) = -Inf, log10(-1.0) = NaN
```

```go
func Log10(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
    if len(args) != 1 {
        return vm.UnitVal(), fmt.Errorf("log10: expected 1 arg, got %d", len(args))
    }
    return vm.FloatVal(math.Log10(toFloat(args[0]))), nil
}
```

### 4.11) exp

```
Signature:  exp(x: Float) -> Float
Arguments:  1
Returns:    e raised to the power x
Edge cases: exp(0.0) = 1.0, exp(1.0) ≈ 2.718...
            exp(-Inf) = 0.0, exp(+Inf) = +Inf
```

```go
func Exp(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
    if len(args) != 1 {
        return vm.UnitVal(), fmt.Errorf("exp: expected 1 arg, got %d", len(args))
    }
    return vm.FloatVal(math.Exp(toFloat(args[0]))), nil
}
```

### 4.12) pi

```
Signature:  pi() -> Float
Arguments:  0
Returns:    The constant π ≈ 3.141592653589793
```

```go
func Pi(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
    if len(args) != 0 {
        return vm.UnitVal(), fmt.Errorf("pi: expected 0 args, got %d", len(args))
    }
    return vm.FloatVal(math.Pi), nil
}
```

### 4.13) e

```
Signature:  e() -> Float
Arguments:  0
Returns:    Euler's number e ≈ 2.718281828459045
```

```go
func E(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
    if len(args) != 0 {
        return vm.UnitVal(), fmt.Errorf("e: expected 0 args, got %d", len(args))
    }
    return vm.FloatVal(math.E), nil
}
```

### 4.14) gcd

```
Signature:  gcd(a: Int, b: Int) -> Int
Arguments:  2 (both must be Int)
Returns:    Greatest common divisor of |a| and |b|
Edge cases: gcd(0, 0) = 0, gcd(n, 0) = |n|, gcd(0, n) = |n|
            gcd(-12, 8) = 4 (absolute values used)
```

```go
func Gcd(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
    if len(args) != 2 {
        return vm.UnitVal(), fmt.Errorf("gcd: expected 2 args, got %d", len(args))
    }
    if args[0].Tag != vm.TagInt || args[1].Tag != vm.TagInt {
        return vm.UnitVal(), fmt.Errorf("gcd: both arguments must be Int")
    }
    a := args[0].AsInt()
    b := args[1].AsInt()
    if a < 0 { a = -a }
    if b < 0 { b = -b }
    for b != 0 {
        a, b = b, a%b
    }
    return vm.IntVal(a), nil
}
```

### 4.15) lcm

```
Signature:  lcm(a: Int, b: Int) -> Int
Arguments:  2 (both must be Int)
Returns:    Least common multiple of |a| and |b|
Edge cases: lcm(0, n) = 0, lcm(n, 0) = 0
```

```go
func Lcm(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
    if len(args) != 2 {
        return vm.UnitVal(), fmt.Errorf("lcm: expected 2 args, got %d", len(args))
    }
    if args[0].Tag != vm.TagInt || args[1].Tag != vm.TagInt {
        return vm.UnitVal(), fmt.Errorf("lcm: both arguments must be Int")
    }
    a := args[0].AsInt()
    b := args[1].AsInt()
    if a < 0 { a = -a }
    if b < 0 { b = -b }
    if a == 0 || b == 0 {
        return vm.IntVal(0), nil
    }
    // gcd via Euclidean algorithm
    ga, gb := a, b
    for gb != 0 {
        ga, gb = gb, ga%gb
    }
    return vm.IntVal(a / ga * b), nil
}
```

### 4.16) clamp

```
Signature:  clamp(x: Int|Float, lo: Int|Float, hi: Int|Float) -> Int|Float
Arguments:  3
Returns:    x clamped to range [lo, hi]
            If all args are Int, returns Int. Otherwise returns Float.
Edge cases: clamp(5, 0, 10) = 5, clamp(-5, 0, 10) = 0, clamp(15, 0, 10) = 10
            clamp(lo=5, hi=3) where lo > hi: returns lo (not undefined)
```

```go
func Clamp(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
    if len(args) != 3 {
        return vm.UnitVal(), fmt.Errorf("clamp: expected 3 args, got %d", len(args))
    }
    // If all Int, use Int comparison
    if args[0].Tag == vm.TagInt && args[1].Tag == vm.TagInt && args[2].Tag == vm.TagInt {
        x := args[0].AsInt()
        lo := args[1].AsInt()
        hi := args[2].AsInt()
        if x < lo { return vm.IntVal(lo), nil }
        if x > hi { return vm.IntVal(hi), nil }
        return vm.IntVal(x), nil
    }
    // Otherwise use Float
    x := toFloat(args[0])
    lo := toFloat(args[1])
    hi := toFloat(args[2])
    if x < lo { return vm.FloatVal(lo), nil }
    if x > hi { return vm.FloatVal(hi), nil }
    return vm.FloatVal(x), nil
}
```

---

## 5) Time + Random + Conversion Stdlib (8 functions)

These go in a new file `pkg/stdlib/time_ops.go` (time and random
functions) and updates to `pkg/stdlib/core.go` (conversions).

### Registration

Add to `RegisterAll` in `core.go`:

```go
// Time (time_ops.go)
r.Register("time_now_ms", TimeNowMs)
r.Register("sleep_ms", SleepMs)

// Random (time_ops.go)
r.Register("random_seed", RandomSeed)
r.Register("random_shuffle", RandomShuffle)
r.Register("random_choice", RandomChoice)

// Conversions (core.go)
r.Register("bool_to_string", BoolToString)
r.Register("char_to_int", CharToInt)
r.Register("int_to_char", IntToChar)
```

### 5.1) time_now_ms

```
Signature:  time_now_ms() -> Int
Arguments:  0
Returns:    Current Unix time in milliseconds (Int)
```

```go
func TimeNowMs(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
    if len(args) != 0 {
        return vm.UnitVal(), fmt.Errorf("time_now_ms: expected 0 args, got %d", len(args))
    }
    ms := time.Now().UnixMilli()
    return vm.IntVal(ms), nil
}
```

### 5.2) sleep_ms

```
Signature:  sleep_ms(ms: Int) -> Unit
Arguments:  1 (milliseconds to sleep, must be non-negative)
Returns:    Unit
Errors:     Runtime error if ms < 0
Note:       This was mandated by the original spec but never implemented.
```

```go
func SleepMs(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
    if len(args) != 1 {
        return vm.UnitVal(), fmt.Errorf("sleep_ms: expected 1 arg, got %d", len(args))
    }
    if args[0].Tag != vm.TagInt {
        return vm.UnitVal(), fmt.Errorf("sleep_ms: argument must be Int")
    }
    ms := args[0].AsInt()
    if ms < 0 {
        return vm.UnitVal(), fmt.Errorf("sleep_ms: duration must be non-negative, got %d", ms)
    }
    time.Sleep(time.Duration(ms) * time.Millisecond)
    return vm.UnitVal(), nil
}
```

### 5.3) random_seed

```
Signature:  random_seed(seed: Int) -> Unit
Arguments:  1 (seed value)
Returns:    Unit
Note:       Seeds the global PRNG used by random_int, random_float,
            random_shuffle, random_choice.  Uses a package-level
            *rand.Rand initialized with the seed.
```

```go
var rng *rand.Rand
var rngMu sync.Mutex

func init() {
    rng = rand.New(rand.NewSource(time.Now().UnixNano()))
}

func RandomSeed(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
    if len(args) != 1 {
        return vm.UnitVal(), fmt.Errorf("random_seed: expected 1 arg, got %d", len(args))
    }
    if args[0].Tag != vm.TagInt {
        return vm.UnitVal(), fmt.Errorf("random_seed: argument must be Int")
    }
    seed := args[0].AsInt()
    rngMu.Lock()
    rng = rand.New(rand.NewSource(seed))
    rngMu.Unlock()
    return vm.UnitVal(), nil
}
```

**Important:** The existing `RandomInt` and `RandomFloat` in
`math_ops.go` should be updated to use this shared `rng` instead of
`math/rand` global state so that `random_seed` controls all random
output.  If the existing functions use `rand.Intn()` directly (global),
refactor them to use the shared `rng`.

### 5.4) random_shuffle

```
Signature:  random_shuffle(arr: [Any]) -> [Any]
Arguments:  1 (array)
Returns:    A NEW array with elements in random order (Fisher-Yates shuffle)
            Original array is NOT modified.
```

```go
func RandomShuffle(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
    if len(args) != 1 {
        return vm.UnitVal(), fmt.Errorf("random_shuffle: expected 1 arg, got %d", len(args))
    }
    a, err := resolveArray(args[0], heap)
    if err != nil {
        return vm.UnitVal(), fmt.Errorf("random_shuffle: %w", err)
    }
    // Copy elements
    shuffled := make([]vm.Value, len(a.Elements))
    copy(shuffled, a.Elements)
    // Fisher-Yates shuffle
    rngMu.Lock()
    for i := len(shuffled) - 1; i > 0; i-- {
        j := rng.Intn(i + 1)
        shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
    }
    rngMu.Unlock()
    idx := heap.AllocArray(shuffled)
    return vm.ObjVal(idx), nil
}
```

### 5.5) random_choice

```
Signature:  random_choice(arr: [Any]) -> Any
Arguments:  1 (non-empty array)
Returns:    A randomly selected element from the array
Errors:     Runtime error if array is empty
```

```go
func RandomChoice(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
    if len(args) != 1 {
        return vm.UnitVal(), fmt.Errorf("random_choice: expected 1 arg, got %d", len(args))
    }
    a, err := resolveArray(args[0], heap)
    if err != nil {
        return vm.UnitVal(), fmt.Errorf("random_choice: %w", err)
    }
    if len(a.Elements) == 0 {
        return vm.UnitVal(), fmt.Errorf("random_choice: array is empty")
    }
    rngMu.Lock()
    idx := rng.Intn(len(a.Elements))
    rngMu.Unlock()
    return a.Elements[idx], nil
}
```

### 5.6) bool_to_string

```
Signature:  bool_to_string(b: Bool) -> String
Arguments:  1 (Bool)
Returns:    "true" or "false" as a heap-allocated String
Errors:     Runtime error if argument is not Bool
```

```go
func BoolToString(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
    if len(args) != 1 {
        return vm.UnitVal(), fmt.Errorf("bool_to_string: expected 1 arg, got %d", len(args))
    }
    if args[0].Tag != vm.TagBool {
        return vm.UnitVal(), fmt.Errorf("bool_to_string: argument must be Bool")
    }
    var s string
    if args[0].AsBool() {
        s = "true"
    } else {
        s = "false"
    }
    idx := heap.AllocString(s)
    return vm.ObjVal(idx), nil
}
```

### 5.7) char_to_int

```
Signature:  char_to_int(c: Char) -> Int
Arguments:  1 (Char)
Returns:    Unicode codepoint as Int (e.g., 'A' → 65)
Errors:     Runtime error if argument is not Char
```

```go
func CharToInt(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
    if len(args) != 1 {
        return vm.UnitVal(), fmt.Errorf("char_to_int: expected 1 arg, got %d", len(args))
    }
    if args[0].Tag != vm.TagChar {
        return vm.UnitVal(), fmt.Errorf("char_to_int: argument must be Char")
    }
    return vm.IntVal(int64(args[0].AsChar())), nil
}
```

### 5.8) int_to_char

```
Signature:  int_to_char(n: Int) -> Char
Arguments:  1 (Int)
Returns:    Character with Unicode codepoint n (e.g., 65 → 'A')
Errors:     Runtime error if argument is not Int
            Runtime error if n < 0 or n > 0x10FFFF (invalid codepoint)
            Runtime error if n is a surrogate (0xD800-0xDFFF)
```

```go
func IntToChar(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
    if len(args) != 1 {
        return vm.UnitVal(), fmt.Errorf("int_to_char: expected 1 arg, got %d", len(args))
    }
    if args[0].Tag != vm.TagInt {
        return vm.UnitVal(), fmt.Errorf("int_to_char: argument must be Int")
    }
    n := args[0].AsInt()
    if n < 0 || n > 0x10FFFF {
        return vm.UnitVal(), fmt.Errorf("int_to_char: codepoint %d out of range [0, 0x10FFFF]", n)
    }
    r := rune(n)
    if !utf8.ValidRune(r) {
        return vm.UnitVal(), fmt.Errorf("int_to_char: codepoint %d is not a valid Unicode scalar value", n)
    }
    return vm.CharVal(r), nil
}
```

---

## 6) Array Stdlib Expansion (12 functions)

All go in `pkg/stdlib/array_ops.go`.

### Registration

Add to `RegisterAll` in `core.go`:

```go
// Additional array operations (array_ops.go)
r.Register("array_find", ArrayFind)
r.Register("array_any", ArrayAny)
r.Register("array_all", ArrayAll)
r.Register("array_sum", ArraySum)
r.Register("array_min", ArrayMin)
r.Register("array_max", ArrayMax)
r.Register("array_take", ArrayTake)
r.Register("array_drop", ArrayDrop)
r.Register("array_chunk", ArrayChunk)
r.Register("array_unique", ArrayUnique)
r.Register("array_join", ArrayJoin)
r.Register("array_slice", ArraySlice)
```

### 6.1) array_find

```
Signature:  array_find(arr: [T], predicate: (T) -> Bool) -> Result<T, String>
Arguments:  2 (array, predicate function)
Returns:    Result — Ok(first element where predicate is true)
                     Err("not found") if no element matches
Uses:       CallbackInvoker
```

```go
func ArrayFind(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
    if len(args) != 2 {
        return vm.UnitVal(), fmt.Errorf("array_find: expected 2 args, got %d", len(args))
    }
    a, err := resolveArray(args[0], heap)
    if err != nil {
        return vm.UnitVal(), fmt.Errorf("array_find: %w", err)
    }
    fn := args[1]
    if CallbackInvoker == nil {
        return vm.UnitVal(), fmt.Errorf("array_find: no callback invoker registered")
    }
    for _, elem := range a.Elements {
        result, callErr := CallbackInvoker(fn, []vm.Value{elem}, heap)
        if callErr != nil {
            return vm.UnitVal(), fmt.Errorf("array_find: callback error: %w", callErr)
        }
        if result.IsTruthy() {
            return makeResultOk(elem, heap), nil
        }
    }
    return makeResultErr("not found", heap), nil
}
```

### 6.2) array_any

```
Signature:  array_any(arr: [T], predicate: (T) -> Bool) -> Bool
Arguments:  2 (array, predicate)
Returns:    true if ANY element satisfies predicate, false otherwise
            Empty array → false (vacuously)
Uses:       CallbackInvoker
```

```go
func ArrayAny(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
    if len(args) != 2 {
        return vm.UnitVal(), fmt.Errorf("array_any: expected 2 args, got %d", len(args))
    }
    a, err := resolveArray(args[0], heap)
    if err != nil {
        return vm.UnitVal(), fmt.Errorf("array_any: %w", err)
    }
    fn := args[1]
    if CallbackInvoker == nil {
        return vm.UnitVal(), fmt.Errorf("array_any: no callback invoker registered")
    }
    for _, elem := range a.Elements {
        result, callErr := CallbackInvoker(fn, []vm.Value{elem}, heap)
        if callErr != nil {
            return vm.UnitVal(), fmt.Errorf("array_any: callback error: %w", callErr)
        }
        if result.IsTruthy() {
            return vm.BoolVal(true), nil
        }
    }
    return vm.BoolVal(false), nil
}
```

### 6.3) array_all

```
Signature:  array_all(arr: [T], predicate: (T) -> Bool) -> Bool
Arguments:  2 (array, predicate)
Returns:    true if ALL elements satisfy predicate, false otherwise
            Empty array → true (vacuously)
Uses:       CallbackInvoker
```

```go
func ArrayAll(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
    if len(args) != 2 {
        return vm.UnitVal(), fmt.Errorf("array_all: expected 2 args, got %d", len(args))
    }
    a, err := resolveArray(args[0], heap)
    if err != nil {
        return vm.UnitVal(), fmt.Errorf("array_all: %w", err)
    }
    fn := args[1]
    if CallbackInvoker == nil {
        return vm.UnitVal(), fmt.Errorf("array_all: no callback invoker registered")
    }
    for _, elem := range a.Elements {
        result, callErr := CallbackInvoker(fn, []vm.Value{elem}, heap)
        if callErr != nil {
            return vm.UnitVal(), fmt.Errorf("array_all: callback error: %w", callErr)
        }
        if !result.IsTruthy() {
            return vm.BoolVal(false), nil
        }
    }
    return vm.BoolVal(true), nil
}
```

### 6.4) array_sum

```
Signature:  array_sum(arr: [Int|Float]) -> Int|Float
Arguments:  1 (array of numeric values)
Returns:    Sum of all elements.
            If all elements are Int, returns Int.
            If any element is Float, returns Float.
            Empty array → IntVal(0)
Errors:     Runtime error if array contains non-numeric values
```

```go
func ArraySum(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
    if len(args) != 1 {
        return vm.UnitVal(), fmt.Errorf("array_sum: expected 1 arg, got %d", len(args))
    }
    a, err := resolveArray(args[0], heap)
    if err != nil {
        return vm.UnitVal(), fmt.Errorf("array_sum: %w", err)
    }
    if len(a.Elements) == 0 {
        return vm.IntVal(0), nil
    }
    hasFloat := false
    for _, e := range a.Elements {
        if e.Tag == vm.TagFloat {
            hasFloat = true
        } else if e.Tag != vm.TagInt {
            return vm.UnitVal(), fmt.Errorf("array_sum: non-numeric element with tag=%d", e.Tag)
        }
    }
    if hasFloat {
        sum := 0.0
        for _, e := range a.Elements {
            sum += toFloat(e)
        }
        return vm.FloatVal(sum), nil
    }
    var sum int64
    for _, e := range a.Elements {
        sum += e.AsInt()
    }
    return vm.IntVal(sum), nil
}
```

### 6.5) array_min

```
Signature:  array_min(arr: [Int|Float]) -> Int|Float
Arguments:  1 (non-empty array of numeric values)
Returns:    Minimum element (preserves type if all same type)
Errors:     Runtime error if array is empty
            Runtime error if array contains non-numeric values
```

```go
func ArrayMin(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
    if len(args) != 1 {
        return vm.UnitVal(), fmt.Errorf("array_min: expected 1 arg, got %d", len(args))
    }
    a, err := resolveArray(args[0], heap)
    if err != nil {
        return vm.UnitVal(), fmt.Errorf("array_min: %w", err)
    }
    if len(a.Elements) == 0 {
        return vm.UnitVal(), fmt.Errorf("array_min: array is empty")
    }
    minVal := a.Elements[0]
    for _, e := range a.Elements[1:] {
        if compareValues(e, minVal, heap) < 0 {
            minVal = e
        }
    }
    return minVal, nil
}
```

### 6.6) array_max

```
Signature:  array_max(arr: [Int|Float]) -> Int|Float
Arguments:  1 (non-empty array of numeric values)
Returns:    Maximum element
Errors:     Runtime error if array is empty
            Runtime error if array contains non-numeric values
```

```go
func ArrayMax(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
    if len(args) != 1 {
        return vm.UnitVal(), fmt.Errorf("array_max: expected 1 arg, got %d", len(args))
    }
    a, err := resolveArray(args[0], heap)
    if err != nil {
        return vm.UnitVal(), fmt.Errorf("array_max: %w", err)
    }
    if len(a.Elements) == 0 {
        return vm.UnitVal(), fmt.Errorf("array_max: array is empty")
    }
    maxVal := a.Elements[0]
    for _, e := range a.Elements[1:] {
        if compareValues(e, maxVal, heap) > 0 {
            maxVal = e
        }
    }
    return maxVal, nil
}
```

### 6.7) array_take

```
Signature:  array_take(arr: [T], n: Int) -> [T]
Arguments:  2 (array, count)
Returns:    New array with first min(n, len(arr)) elements
            If n <= 0, returns empty array
            If n >= len(arr), returns copy of entire array
```

```go
func ArrayTake(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
    if len(args) != 2 {
        return vm.UnitVal(), fmt.Errorf("array_take: expected 2 args, got %d", len(args))
    }
    a, err := resolveArray(args[0], heap)
    if err != nil {
        return vm.UnitVal(), fmt.Errorf("array_take: %w", err)
    }
    if args[1].Tag != vm.TagInt {
        return vm.UnitVal(), fmt.Errorf("array_take: second argument must be Int")
    }
    n := int(args[1].AsInt())
    if n <= 0 {
        idx := heap.AllocArray([]vm.Value{})
        return vm.ObjVal(idx), nil
    }
    if n > len(a.Elements) {
        n = len(a.Elements)
    }
    taken := make([]vm.Value, n)
    copy(taken, a.Elements[:n])
    idx := heap.AllocArray(taken)
    return vm.ObjVal(idx), nil
}
```

### 6.8) array_drop

```
Signature:  array_drop(arr: [T], n: Int) -> [T]
Arguments:  2 (array, count)
Returns:    New array with first n elements removed
            If n <= 0, returns copy of entire array
            If n >= len(arr), returns empty array
```

```go
func ArrayDrop(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
    if len(args) != 2 {
        return vm.UnitVal(), fmt.Errorf("array_drop: expected 2 args, got %d", len(args))
    }
    a, err := resolveArray(args[0], heap)
    if err != nil {
        return vm.UnitVal(), fmt.Errorf("array_drop: %w", err)
    }
    if args[1].Tag != vm.TagInt {
        return vm.UnitVal(), fmt.Errorf("array_drop: second argument must be Int")
    }
    n := int(args[1].AsInt())
    if n <= 0 {
        n = 0
    }
    if n >= len(a.Elements) {
        idx := heap.AllocArray([]vm.Value{})
        return vm.ObjVal(idx), nil
    }
    dropped := make([]vm.Value, len(a.Elements)-n)
    copy(dropped, a.Elements[n:])
    idx := heap.AllocArray(dropped)
    return vm.ObjVal(idx), nil
}
```

### 6.9) array_chunk

```
Signature:  array_chunk(arr: [T], size: Int) -> [[T]]
Arguments:  2 (array, chunk size)
Returns:    Array of arrays, each with at most `size` elements.
            Last chunk may be smaller.
Errors:     Runtime error if size <= 0
            Empty array → [[]] (array containing empty array) — no,
            actually empty array → [] (empty result array)
```

```go
func ArrayChunk(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
    if len(args) != 2 {
        return vm.UnitVal(), fmt.Errorf("array_chunk: expected 2 args, got %d", len(args))
    }
    a, err := resolveArray(args[0], heap)
    if err != nil {
        return vm.UnitVal(), fmt.Errorf("array_chunk: %w", err)
    }
    if args[1].Tag != vm.TagInt {
        return vm.UnitVal(), fmt.Errorf("array_chunk: second argument must be Int")
    }
    size := int(args[1].AsInt())
    if size <= 0 {
        return vm.UnitVal(), fmt.Errorf("array_chunk: chunk size must be positive, got %d", size)
    }
    if len(a.Elements) == 0 {
        idx := heap.AllocArray([]vm.Value{})
        return vm.ObjVal(idx), nil
    }
    var chunks []vm.Value
    for i := 0; i < len(a.Elements); i += size {
        end := i + size
        if end > len(a.Elements) {
            end = len(a.Elements)
        }
        chunk := make([]vm.Value, end-i)
        copy(chunk, a.Elements[i:end])
        chunkIdx := heap.AllocArray(chunk)
        chunks = append(chunks, vm.ObjVal(chunkIdx))
    }
    idx := heap.AllocArray(chunks)
    return vm.ObjVal(idx), nil
}
```

### 6.10) array_unique

```
Signature:  array_unique(arr: [T]) -> [T]
Arguments:  1 (array)
Returns:    New array with duplicates removed (first occurrence kept).
            Order is preserved.
            Uses Value.Equal() for equality comparison.
```

```go
func ArrayUnique(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
    if len(args) != 1 {
        return vm.UnitVal(), fmt.Errorf("array_unique: expected 1 arg, got %d", len(args))
    }
    a, err := resolveArray(args[0], heap)
    if err != nil {
        return vm.UnitVal(), fmt.Errorf("array_unique: %w", err)
    }
    var unique []vm.Value
    for _, elem := range a.Elements {
        found := false
        for _, u := range unique {
            if elem.Equal(u, heap) {
                found = true
                break
            }
        }
        if !found {
            unique = append(unique, elem)
        }
    }
    idx := heap.AllocArray(unique)
    return vm.ObjVal(idx), nil
}
```

### 6.11) array_join

```
Signature:  array_join(arr: [String], separator: String) -> String
Arguments:  2 (array of strings, separator string)
Returns:    Single string with all elements joined by separator
Errors:     Runtime error if array elements are not strings
            Empty array → "" (empty string)
```

```go
func ArrayJoin(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
    if len(args) != 2 {
        return vm.UnitVal(), fmt.Errorf("array_join: expected 2 args, got %d", len(args))
    }
    a, err := resolveArray(args[0], heap)
    if err != nil {
        return vm.UnitVal(), fmt.Errorf("array_join: %w", err)
    }
    sep, err := resolveString(args[1], heap)
    if err != nil {
        return vm.UnitVal(), fmt.Errorf("array_join: separator: %w", err)
    }
    parts := make([]string, len(a.Elements))
    for i, elem := range a.Elements {
        s, err := resolveString(elem, heap)
        if err != nil {
            return vm.UnitVal(), fmt.Errorf("array_join: element %d: %w", i, err)
        }
        parts[i] = s
    }
    result := strings.Join(parts, sep)
    idx := heap.AllocString(result)
    return vm.ObjVal(idx), nil
}
```

### 6.12) array_slice

```
Signature:  array_slice(arr: [T], start: Int, end: Int) -> [T]
Arguments:  3 (array, start index, end index — end is exclusive)
Returns:    New array containing elements from start to end-1
            Indices are clamped to valid range (no error on out-of-bounds)
            Start and end can be negative (count from end):
              -1 = last element, -2 = second to last, etc.
```

```go
func ArraySlice(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
    if len(args) != 3 {
        return vm.UnitVal(), fmt.Errorf("array_slice: expected 3 args, got %d", len(args))
    }
    a, err := resolveArray(args[0], heap)
    if err != nil {
        return vm.UnitVal(), fmt.Errorf("array_slice: %w", err)
    }
    if args[1].Tag != vm.TagInt || args[2].Tag != vm.TagInt {
        return vm.UnitVal(), fmt.Errorf("array_slice: indices must be Int")
    }
    n := len(a.Elements)
    start := int(args[1].AsInt())
    end := int(args[2].AsInt())

    // Negative index handling
    if start < 0 { start = n + start }
    if end < 0 { end = n + end }

    // Clamp
    if start < 0 { start = 0 }
    if end > n { end = n }
    if start >= end {
        idx := heap.AllocArray([]vm.Value{})
        return vm.ObjVal(idx), nil
    }

    sliced := make([]vm.Value, end-start)
    copy(sliced, a.Elements[start:end])
    idx := heap.AllocArray(sliced)
    return vm.ObjVal(idx), nil
}
```

---

## 7) String Stdlib Expansion (6 functions)

All go in `pkg/stdlib/string_ops.go`.

### Registration

Add to `RegisterAll` in `core.go`:

```go
// Additional string operations (string_ops.go)
r.Register("string_index_of", StringIndexOf)
r.Register("string_repeat", StringRepeat)
r.Register("string_pad_left", StringPadLeft)
r.Register("string_pad_right", StringPadRight)
r.Register("string_bytes", StringBytes)
r.Register("string_join", StringJoin)
```

### 7.1) string_index_of

```
Signature:  string_index_of(haystack: String, needle: String) -> Int
Arguments:  2
Returns:    Index (in Unicode codepoints, not bytes) of first occurrence
            of needle in haystack.
            Returns -1 if not found.
```

```go
func StringIndexOf(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
    if len(args) != 2 {
        return vm.UnitVal(), fmt.Errorf("string_index_of: expected 2 args, got %d", len(args))
    }
    haystack, err := resolveString(args[0], heap)
    if err != nil {
        return vm.UnitVal(), fmt.Errorf("string_index_of: %w", err)
    }
    needle, err := resolveString(args[1], heap)
    if err != nil {
        return vm.UnitVal(), fmt.Errorf("string_index_of: %w", err)
    }

    // Find byte index first
    byteIdx := strings.Index(haystack, needle)
    if byteIdx < 0 {
        return vm.IntVal(-1), nil
    }
    // Convert byte index to codepoint index
    cpIdx := utf8.RuneCountInString(haystack[:byteIdx])
    return vm.IntVal(int64(cpIdx)), nil
}
```

**Edge cases:**
- `string_index_of("hello", "")` → 0 (empty string found at start)
- `string_index_of("", "x")` → -1
- `string_index_of("café", "é")` → 3 (codepoint index, not byte index)

### 7.2) string_repeat

```
Signature:  string_repeat(s: String, n: Int) -> String
Arguments:  2 (string, repeat count)
Returns:    String repeated n times
Errors:     Runtime error if n < 0
            n = 0 → "" (empty string)
```

```go
func StringRepeat(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
    if len(args) != 2 {
        return vm.UnitVal(), fmt.Errorf("string_repeat: expected 2 args, got %d", len(args))
    }
    s, err := resolveString(args[0], heap)
    if err != nil {
        return vm.UnitVal(), fmt.Errorf("string_repeat: %w", err)
    }
    if args[1].Tag != vm.TagInt {
        return vm.UnitVal(), fmt.Errorf("string_repeat: second argument must be Int")
    }
    n := int(args[1].AsInt())
    if n < 0 {
        return vm.UnitVal(), fmt.Errorf("string_repeat: count must be non-negative, got %d", n)
    }
    result := strings.Repeat(s, n)
    idx := heap.AllocString(result)
    return vm.ObjVal(idx), nil
}
```

### 7.3) string_pad_left

```
Signature:  string_pad_left(s: String, width: Int, pad_char: Char) -> String
Arguments:  3 (string, target width in codepoints, padding character)
Returns:    String padded on the left to reach width.
            If string is already >= width, returns original string unchanged.
```

```go
func StringPadLeft(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
    if len(args) != 3 {
        return vm.UnitVal(), fmt.Errorf("string_pad_left: expected 3 args, got %d", len(args))
    }
    s, err := resolveString(args[0], heap)
    if err != nil {
        return vm.UnitVal(), fmt.Errorf("string_pad_left: %w", err)
    }
    if args[1].Tag != vm.TagInt {
        return vm.UnitVal(), fmt.Errorf("string_pad_left: width must be Int")
    }
    if args[2].Tag != vm.TagChar {
        return vm.UnitVal(), fmt.Errorf("string_pad_left: pad_char must be Char")
    }
    width := int(args[1].AsInt())
    padChar := args[2].AsChar()
    runeLen := utf8.RuneCountInString(s)
    if runeLen >= width {
        return args[0], nil  // return original string (no allocation)
    }
    padding := strings.Repeat(string(padChar), width-runeLen)
    result := padding + s
    idx := heap.AllocString(result)
    return vm.ObjVal(idx), nil
}
```

### 7.4) string_pad_right

```
Signature:  string_pad_right(s: String, width: Int, pad_char: Char) -> String
Arguments:  3 (string, target width in codepoints, padding character)
Returns:    String padded on the right to reach width.
            If string is already >= width, returns original string unchanged.
```

```go
func StringPadRight(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
    if len(args) != 3 {
        return vm.UnitVal(), fmt.Errorf("string_pad_right: expected 3 args, got %d", len(args))
    }
    s, err := resolveString(args[0], heap)
    if err != nil {
        return vm.UnitVal(), fmt.Errorf("string_pad_right: %w", err)
    }
    if args[1].Tag != vm.TagInt {
        return vm.UnitVal(), fmt.Errorf("string_pad_right: width must be Int")
    }
    if args[2].Tag != vm.TagChar {
        return vm.UnitVal(), fmt.Errorf("string_pad_right: pad_char must be Char")
    }
    width := int(args[1].AsInt())
    padChar := args[2].AsChar()
    runeLen := utf8.RuneCountInString(s)
    if runeLen >= width {
        return args[0], nil
    }
    padding := strings.Repeat(string(padChar), width-runeLen)
    result := s + padding
    idx := heap.AllocString(result)
    return vm.ObjVal(idx), nil
}
```

### 7.5) string_bytes

```
Signature:  string_bytes(s: String) -> [Int]
Arguments:  1 (string)
Returns:    Array of byte values (each 0-255) of the UTF-8 encoding
```

```go
func StringBytes(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
    if len(args) != 1 {
        return vm.UnitVal(), fmt.Errorf("string_bytes: expected 1 arg, got %d", len(args))
    }
    s, err := resolveString(args[0], heap)
    if err != nil {
        return vm.UnitVal(), fmt.Errorf("string_bytes: %w", err)
    }
    bytes := []byte(s)
    elems := make([]vm.Value, len(bytes))
    for i, b := range bytes {
        elems[i] = vm.IntVal(int64(b))
    }
    idx := heap.AllocArray(elems)
    return vm.ObjVal(idx), nil
}
```

### 7.6) string_join

```
Signature:  string_join(parts: [String], separator: String) -> String
Arguments:  2 (array of strings, separator)
Returns:    Single string with all parts joined by separator
Note:       This is an alias / alternative name for array_join with
            reversed argument order for discoverability.
            Actually — keep the same argument order as array_join:
            string_join(parts, sep).
```

```go
func StringJoin(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
    // Delegate to ArrayJoin — same signature and behavior
    return ArrayJoin(args, heap)
}
```

---

## 8) File System Stdlib Expansion (8 functions)

All go in `pkg/stdlib/io.go`.

### Registration

Add to `RegisterAll` in `core.go`:

```go
// File system operations (io.go)
r.Register("file_exists", FileExists)
r.Register("dir_list", DirList)
r.Register("dir_create", DirCreate)
r.Register("path_join", PathJoin)
r.Register("path_dirname", PathDirname)
r.Register("path_basename", PathBasename)
r.Register("path_extension", PathExtension)
r.Register("file_size", FileSize)
```

### 8.1) file_exists

```
Signature:  file_exists(path: String) -> Bool
Arguments:  1 (file path)
Returns:    true if path exists (file or directory), false otherwise
            Does NOT distinguish between files and directories.
```

```go
func FileExists(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
    if len(args) != 1 {
        return vm.UnitVal(), fmt.Errorf("file_exists: expected 1 arg, got %d", len(args))
    }
    path, err := resolveString(args[0], heap)
    if err != nil {
        return vm.UnitVal(), fmt.Errorf("file_exists: %w", err)
    }
    _, statErr := os.Stat(path)
    return vm.BoolVal(statErr == nil), nil
}
```

### 8.2) dir_list

```
Signature:  dir_list(path: String) -> Result<[String], String>
Arguments:  1 (directory path)
Returns:    Result — Ok(array of entry names) or Err(error message)
            Entries include both files and subdirectories.
            Returns names only (not full paths).
```

```go
func DirList(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
    if len(args) != 1 {
        return vm.UnitVal(), fmt.Errorf("dir_list: expected 1 arg, got %d", len(args))
    }
    path, err := resolveString(args[0], heap)
    if err != nil {
        return vm.UnitVal(), fmt.Errorf("dir_list: %w", err)
    }
    entries, readErr := os.ReadDir(path)
    if readErr != nil {
        return makeResultErr(readErr.Error(), heap), nil
    }
    names := make([]vm.Value, len(entries))
    for i, e := range entries {
        idx := heap.AllocString(e.Name())
        names[i] = vm.ObjVal(idx)
    }
    arrIdx := heap.AllocArray(names)
    return makeResultOk(vm.ObjVal(arrIdx), heap), nil
}
```

### 8.3) dir_create

```
Signature:  dir_create(path: String) -> Result<Unit, String>
Arguments:  1 (directory path)
Returns:    Result — Ok(Unit) or Err(error message)
            Creates all intermediate directories (like mkdir -p).
```

```go
func DirCreate(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
    if len(args) != 1 {
        return vm.UnitVal(), fmt.Errorf("dir_create: expected 1 arg, got %d", len(args))
    }
    path, err := resolveString(args[0], heap)
    if err != nil {
        return vm.UnitVal(), fmt.Errorf("dir_create: %w", err)
    }
    mkErr := os.MkdirAll(path, 0755)
    if mkErr != nil {
        return makeResultErr(mkErr.Error(), heap), nil
    }
    return makeResultOk(vm.UnitVal(), heap), nil
}
```

### 8.4) path_join

```
Signature:  path_join(parts: [String]) -> String
Arguments:  1 (array of path components)
Returns:    Joined path using OS-appropriate separator
```

```go
func PathJoin(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
    if len(args) != 1 {
        return vm.UnitVal(), fmt.Errorf("path_join: expected 1 arg, got %d", len(args))
    }
    a, err := resolveArray(args[0], heap)
    if err != nil {
        return vm.UnitVal(), fmt.Errorf("path_join: %w", err)
    }
    parts := make([]string, len(a.Elements))
    for i, elem := range a.Elements {
        s, err := resolveString(elem, heap)
        if err != nil {
            return vm.UnitVal(), fmt.Errorf("path_join: element %d: %w", i, err)
        }
        parts[i] = s
    }
    result := filepath.Join(parts...)
    idx := heap.AllocString(result)
    return vm.ObjVal(idx), nil
}
```

### 8.5) path_dirname

```
Signature:  path_dirname(path: String) -> String
Arguments:  1 (file path)
Returns:    Directory component of path
            path_dirname("/usr/local/bin/ryx") → "/usr/local/bin"
            path_dirname("file.txt") → "."
```

```go
func PathDirname(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
    if len(args) != 1 {
        return vm.UnitVal(), fmt.Errorf("path_dirname: expected 1 arg, got %d", len(args))
    }
    path, err := resolveString(args[0], heap)
    if err != nil {
        return vm.UnitVal(), fmt.Errorf("path_dirname: %w", err)
    }
    dir := filepath.Dir(path)
    idx := heap.AllocString(dir)
    return vm.ObjVal(idx), nil
}
```

### 8.6) path_basename

```
Signature:  path_basename(path: String) -> String
Arguments:  1 (file path)
Returns:    Final component of path (file name with extension)
            path_basename("/usr/local/bin/ryx") → "ryx"
            path_basename("/tmp/data.csv") → "data.csv"
```

```go
func PathBasename(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
    if len(args) != 1 {
        return vm.UnitVal(), fmt.Errorf("path_basename: expected 1 arg, got %d", len(args))
    }
    path, err := resolveString(args[0], heap)
    if err != nil {
        return vm.UnitVal(), fmt.Errorf("path_basename: %w", err)
    }
    base := filepath.Base(path)
    idx := heap.AllocString(base)
    return vm.ObjVal(idx), nil
}
```

### 8.7) path_extension

```
Signature:  path_extension(path: String) -> String
Arguments:  1 (file path)
Returns:    Extension including the dot, or empty string if none
            path_extension("data.csv") → ".csv"
            path_extension("Makefile") → ""
            path_extension("archive.tar.gz") → ".gz"
```

```go
func PathExtension(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
    if len(args) != 1 {
        return vm.UnitVal(), fmt.Errorf("path_extension: expected 1 arg, got %d", len(args))
    }
    path, err := resolveString(args[0], heap)
    if err != nil {
        return vm.UnitVal(), fmt.Errorf("path_extension: %w", err)
    }
    ext := filepath.Ext(path)
    idx := heap.AllocString(ext)
    return vm.ObjVal(idx), nil
}
```

### 8.8) file_size

```
Signature:  file_size(path: String) -> Result<Int, String>
Arguments:  1 (file path)
Returns:    Result — Ok(size in bytes) or Err(error message)
```

```go
func FileSize(args []vm.Value, heap *vm.Heap) (vm.Value, error) {
    if len(args) != 1 {
        return vm.UnitVal(), fmt.Errorf("file_size: expected 1 arg, got %d", len(args))
    }
    path, err := resolveString(args[0], heap)
    if err != nil {
        return vm.UnitVal(), fmt.Errorf("file_size: %w", err)
    }
    info, statErr := os.Stat(path)
    if statErr != nil {
        return makeResultErr(statErr.Error(), heap), nil
    }
    return makeResultOk(vm.IntVal(info.Size()), heap), nil
}
```

---

## 9) Test Expectations

### 9.1) Unit Tests — Map Operations

File: `pkg/stdlib/map_ops_test.go` (new)

```go
func TestMapNewAndLen(t *testing.T) {
    heap := vm.NewHeap()
    m, _ := MapNew(nil, heap)
    result, _ := MapLen([]vm.Value{m}, heap)
    assert(result.AsInt() == 0)
}

func TestMapSetAndGet(t *testing.T) {
    heap := vm.NewHeap()
    m, _ := MapNew(nil, heap)

    // Set string key → int value
    keyIdx := heap.AllocString("alice")
    m, _ = MapSet([]vm.Value{m, vm.ObjVal(keyIdx), vm.IntVal(42)}, heap)

    // Get it back
    result, _ := MapGet([]vm.Value{m, vm.ObjVal(keyIdx)}, heap)
    assert(result.AsInt() == 42)

    // Len should be 1
    lenResult, _ := MapLen([]vm.Value{m}, heap)
    assert(lenResult.AsInt() == 1)
}

func TestMapContainsAndDelete(t *testing.T) {
    heap := vm.NewHeap()
    m, _ := MapNew(nil, heap)
    keyIdx := heap.AllocString("key")
    key := vm.ObjVal(keyIdx)

    // Not found initially
    found, _ := MapContains([]vm.Value{m, key}, heap)
    assert(found.AsBool() == false)

    // Set and check
    m, _ = MapSet([]vm.Value{m, key, vm.IntVal(1)}, heap)
    found, _ = MapContains([]vm.Value{m, key}, heap)
    assert(found.AsBool() == true)

    // Delete and check
    m, _ = MapDelete([]vm.Value{m, key}, heap)
    found, _ = MapContains([]vm.Value{m, key}, heap)
    assert(found.AsBool() == false)
}

func TestMapGetMissing(t *testing.T) {
    heap := vm.NewHeap()
    m, _ := MapNew(nil, heap)
    keyIdx := heap.AllocString("missing")
    _, err := MapGet([]vm.Value{m, vm.ObjVal(keyIdx)}, heap)
    assert(err != nil) // should error
    assert(strings.Contains(err.Error(), "key not found"))
}

func TestMapIntKeys(t *testing.T) {
    heap := vm.NewHeap()
    m, _ := MapNew(nil, heap)
    m, _ = MapSet([]vm.Value{m, vm.IntVal(1), vm.IntVal(100)}, heap)
    m, _ = MapSet([]vm.Value{m, vm.IntVal(2), vm.IntVal(200)}, heap)
    result, _ := MapGet([]vm.Value{m, vm.IntVal(1)}, heap)
    assert(result.AsInt() == 100)
}

func TestMapOverwrite(t *testing.T) {
    heap := vm.NewHeap()
    m, _ := MapNew(nil, heap)
    keyIdx := heap.AllocString("x")
    key := vm.ObjVal(keyIdx)
    m, _ = MapSet([]vm.Value{m, key, vm.IntVal(1)}, heap)
    m, _ = MapSet([]vm.Value{m, key, vm.IntVal(2)}, heap)
    result, _ := MapGet([]vm.Value{m, key}, heap)
    assert(result.AsInt() == 2)
    lenResult, _ := MapLen([]vm.Value{m}, heap)
    assert(lenResult.AsInt() == 1) // not 2
}

func TestMapKeysAndValues(t *testing.T) {
    heap := vm.NewHeap()
    m, _ := MapNew(nil, heap)
    m, _ = MapSet([]vm.Value{m, vm.IntVal(1), vm.IntVal(10)}, heap)
    m, _ = MapSet([]vm.Value{m, vm.IntVal(2), vm.IntVal(20)}, heap)

    keysResult, _ := MapKeys([]vm.Value{m}, heap)
    keysArr := heap.Get(keysResult.AsObj()).Data.(*vm.ArrayObj)
    assert(len(keysArr.Elements) == 2)

    valsResult, _ := MapValues([]vm.Value{m}, heap)
    valsArr := heap.Get(valsResult.AsObj()).Data.(*vm.ArrayObj)
    assert(len(valsArr.Elements) == 2)
}

func TestMapMerge(t *testing.T) {
    heap := vm.NewHeap()
    m1, _ := MapNew(nil, heap)
    m2, _ := MapNew(nil, heap)
    k1 := heap.AllocString("a")
    k2 := heap.AllocString("b")
    k3 := heap.AllocString("a") // same key as k1

    m1, _ = MapSet([]vm.Value{m1, vm.ObjVal(k1), vm.IntVal(1)}, heap)
    m2, _ = MapSet([]vm.Value{m2, vm.ObjVal(k2), vm.IntVal(2)}, heap)
    m2, _ = MapSet([]vm.Value{m2, vm.ObjVal(k3), vm.IntVal(99)}, heap) // "a" → 99

    merged, _ := MapMerge([]vm.Value{m1, m2}, heap)
    // "a" should be 99 (m2 wins), "b" should be 2
    result, _ := MapGet([]vm.Value{merged, vm.ObjVal(k1)}, heap)
    assert(result.AsInt() == 99)
}

func TestMapEntries(t *testing.T) {
    heap := vm.NewHeap()
    m, _ := MapNew(nil, heap)
    m, _ = MapSet([]vm.Value{m, vm.IntVal(1), vm.IntVal(10)}, heap)

    entries, _ := MapEntries([]vm.Value{m}, heap)
    arr := heap.Get(entries.AsObj()).Data.(*vm.ArrayObj)
    assert(len(arr.Elements) == 1)
    // Each entry is a tuple (key, value)
    tuple := heap.Get(arr.Elements[0].AsObj()).Data.(*vm.TupleObj)
    assert(tuple.Elements[0].AsInt() == 1)
    assert(tuple.Elements[1].AsInt() == 10)
}
```

### 9.2) Unit Tests — Math Functions

File: `pkg/stdlib/math_ops_test.go` (new or extend existing)

```
sin(0.0)                     → 0.0
sin(pi() / 2.0)              → 1.0  (within 1e-10 tolerance)
cos(0.0)                     → 1.0
cos(pi())                    → -1.0  (within 1e-10)
tan(0.0)                     → 0.0
asin(0.0)                    → 0.0
asin(1.0)                    → pi/2  (within 1e-10)
acos(1.0)                    → 0.0
atan(0.0)                    → 0.0
atan2(1.0, 0.0)              → pi/2  (within 1e-10)
log(1.0)                     → 0.0
log(math.E)                  → 1.0  (within 1e-10)
log2(8.0)                    → 3.0
log10(100.0)                 → 2.0
exp(0.0)                     → 1.0
exp(1.0)                     → math.E  (within 1e-10)
pi()                         → math.Pi
e()                          → math.E
gcd(12, 8)                   → 4
gcd(0, 5)                    → 5
gcd(7, 0)                    → 7
gcd(0, 0)                    → 0
gcd(-12, 8)                  → 4
lcm(4, 6)                    → 12
lcm(0, 5)                    → 0
clamp(5, 0, 10)              → 5  (Int)
clamp(-5, 0, 10)             → 0  (Int)
clamp(15, 0, 10)             → 10 (Int)
clamp(5.5, 0.0, 10.0)        → 5.5 (Float)
```

### 9.3) Unit Tests — Array Expansion

```
array_sum([3,1,4,1,5])       → 14  (Int)
array_sum([1.0, 2.0, 3.0])   → 6.0 (Float)
array_sum([])                 → 0   (Int)
array_min([3,1,4,1,5])       → 1
array_max([3,1,4,1,5])       → 5
array_min([])                 → error "array is empty"
array_any([1,2,3], |x| x>2)  → true
array_any([1,2,3], |x| x>5)  → false
array_any([], |x| true)      → false
array_all([1,2,3], |x| x>0)  → true
array_all([1,2,3], |x| x>1)  → false
array_all([], |x| false)     → true
array_take([1,2,3,4,5], 3)   → [1,2,3]
array_take([1,2], 5)          → [1,2]
array_take([1,2,3], 0)        → []
array_drop([1,2,3,4,5], 2)   → [3,4,5]
array_drop([1,2], 5)          → []
array_chunk([1,2,3,4,5], 2)  → [[1,2],[3,4],[5]]
array_chunk([], 3)            → []
array_chunk([1,2,3], 0)       → error "chunk size must be positive"
array_unique([3,1,4,1,5,3])  → [3,1,4,5]  (preserves first occurrence order)
array_join(["a","b","c"],"-") → "a-b-c"
array_join([], ",")           → ""
array_slice([1,2,3,4,5],1,4) → [2,3,4]
array_slice([1,2,3], -2, 10) → [2,3]  (negative start, clamped end)
array_find([1,2,3], |x| x>1) → Ok(2)  (first match)
array_find([1,2,3], |x| x>5) → Err("not found")
```

### 9.4) Unit Tests — String Expansion

```
string_index_of("hello", "ll")     → 2
string_index_of("hello", "xyz")    → -1
string_index_of("hello", "")       → 0
string_repeat("ab", 3)             → "ababab"
string_repeat("x", 0)              → ""
string_pad_left("42", 5, '0')      → "00042"
string_pad_left("hello", 3, '0')   → "hello"  (already >= width)
string_pad_right("hi", 5, '.')     → "hi..."
string_bytes("ABC")                → [65, 66, 67]
string_join(["a","b","c"], "-")     → "a-b-c"
```

### 9.5) Unit Tests — Time/Random/Conversion

```
time_now_ms()                → positive Int (just check > 0)
sleep_ms(0)                  → Unit (succeeds)
sleep_ms(-1)                 → error "duration must be non-negative"
random_seed(42) then random_int(0,100) → deterministic value
random_shuffle([1,2,3])     → array of length 3 with same elements
random_choice([10,20,30])   → one of {10, 20, 30}
random_choice([])            → error "array is empty"
bool_to_string(true)         → "true"
bool_to_string(false)        → "false"
char_to_int('A')             → 65
char_to_int('0')             → 48
int_to_char(65)              → 'A'
int_to_char(0x1F600)         → '😀' (emoji)
int_to_char(-1)              → error "out of range"
int_to_char(0xD800)          → error "not a valid Unicode scalar"
```

### 9.6) Unit Tests — File System

```
file_exists("/tmp")                → true  (or some known path)
file_exists("/nonexistent/path")   → false
dir_create("/tmp/ryx_test_dir")    → Ok(())
dir_list("/tmp/ryx_test_dir")      → Ok([])  (empty dir)
path_join(["usr", "local", "bin"]) → "usr/local/bin"
path_dirname("/usr/local/bin/ryx") → "/usr/local/bin"
path_basename("/tmp/data.csv")     → "data.csv"
path_extension("data.csv")         → ".csv"
path_extension("Makefile")         → ""
file_size (use a temp file)        → Ok(n) where n matches written bytes
```

### 9.7) Integration Test Programs

#### `tests/testdata/programs/map_operations.ryx`
```ryx
fn main() {
    let mut m = map_new();
    m = map_set(m, "alice", 42);
    m = map_set(m, "bob", 17);
    m = map_set(m, "carol", 99);

    println(map_len(m));
    println(map_get(m, "alice"));
    println(map_get(m, "bob"));
    println(map_contains(m, "carol"));
    println(map_contains(m, "dave"));

    m = map_delete(m, "bob");
    println(map_len(m));
    println(map_contains(m, "bob"));

    // Overwrite
    m = map_set(m, "alice", 100);
    println(map_get(m, "alice"));
}
```

`tests/testdata/programs/map_operations.expected`:
```
3
42
17
true
false
2
false
100
```

#### `tests/testdata/programs/word_counter.ryx`
```ryx
fn main() {
    let words = string_split("the cat sat on the mat the cat", " ");
    let mut counts = map_new();
    let mut i = 0;
    while i < array_len(words) {
        let w = words[i];
        if map_contains(counts, w) {
            let c = map_get(counts, w);
            counts = map_set(counts, w, c + 1);
        } else {
            counts = map_set(counts, w, 1);
        };
        i = i + 1;
    };
    println(map_len(counts));
    println(map_get(counts, "the"));
    println(map_get(counts, "cat"));
    println(map_get(counts, "sat"));
    println(map_get(counts, "on"));
    println(map_get(counts, "mat"));
}
```

`tests/testdata/programs/word_counter.expected`:
```
5
3
2
1
1
1
```

#### `tests/testdata/programs/phone_book.ryx`
```ryx
fn main() {
    let mut book = map_new();
    book = map_set(book, "Alice", "555-0100");
    book = map_set(book, "Bob", "555-0200");
    book = map_set(book, "Carol", "555-0300");

    // Lookup
    println(map_get(book, "Bob"));

    // Update
    book = map_set(book, "Bob", "555-9999");
    println(map_get(book, "Bob"));

    // List all entries
    println(map_len(book));

    // Delete and verify
    book = map_delete(book, "Carol");
    println(map_contains(book, "Carol"));
    println(map_len(book));
}
```

`tests/testdata/programs/phone_book.expected`:
```
555-0200
555-9999
3
false
2
```

#### `tests/testdata/programs/group_by.ryx`
```ryx
fn main() {
    // Group numbers by even/odd
    let nums = [1, 2, 3, 4, 5, 6, 7, 8, 9, 10];
    let mut groups = map_new();
    groups = map_set(groups, "even", []);
    groups = map_set(groups, "odd", []);

    let mut i = 0;
    while i < array_len(nums) {
        let n = nums[i];
        if n % 2 == 0 {
            let arr = map_get(groups, "even");
            let arr2 = array_push(arr, n);
            groups = map_set(groups, "even", arr2);
        } else {
            let arr = map_get(groups, "odd");
            let arr2 = array_push(arr, n);
            groups = map_set(groups, "odd", arr2);
        };
        i = i + 1;
    };

    let evens = map_get(groups, "even");
    let odds = map_get(groups, "odd");
    println(array_len(evens));
    println(array_len(odds));
    println(array_sum(evens));
    println(array_sum(odds));
}
```

`tests/testdata/programs/group_by.expected`:
```
5
5
30
25
```

### 9.8) GC Stress Test for Maps

File: `pkg/gc/gc_test.go` (add test case)

```go
func TestMapGCStress(t *testing.T) {
    h := NewHeap()
    // Allocate many maps with string key/value pairs
    // Keep only some reachable
    // Run full GC
    // Verify reachable maps survive, unreachable maps are collected
    // Verify children (string keys/values) of reachable maps survive
}
```

Test expectations:
- Create 1000 maps with 10 entries each
- Keep references to only 100 maps
- After GC: exactly 100 maps survive
- All keys and values of surviving maps are accessible
- No dangling references

---

## 10) Files to Modify

### New Files

| File | Description |
|------|-------------|
| `pkg/stdlib/map_ops.go` | 12 map stdlib functions + resolveMap helper |
| `pkg/stdlib/map_ops_test.go` | Unit tests for map operations |
| `pkg/stdlib/time_ops.go` | time_now_ms, sleep_ms, random_seed, random_shuffle, random_choice |
| `tests/testdata/programs/map_operations.ryx` | Integration test |
| `tests/testdata/programs/map_operations.expected` | Expected output |
| `tests/testdata/programs/word_counter.ryx` | Integration test |
| `tests/testdata/programs/word_counter.expected` | Expected output |
| `tests/testdata/programs/phone_book.ryx` | Integration test |
| `tests/testdata/programs/phone_book.expected` | Expected output |
| `tests/testdata/programs/group_by.ryx` | Integration test |
| `tests/testdata/programs/group_by.expected` | Expected output |

### Modified Files

| File | Changes |
|------|---------|
| `pkg/vm/value.go` | Add `ObjMap` constant (0x27), `mapEntry` struct, `MapObj` struct, `ComputeHash()` function, `MapObj` methods (Get/Set/Delete/Contains/Keys/Values/Entries) |
| `pkg/vm/heap.go` | Add `AllocMap()` method |
| `pkg/vm/builtins.go` | Add `*MapObj` cases to `Equal()`, `StringValue()`/`displayValue()`, `deepClone()`, `hashValue()`, `deepCompare()` |
| `pkg/gc/gc.go` | Add `KindMap` to `ObjectKind` enum |
| `pkg/gc/write_barrier.go` | Add `BarrierMapSet()` convenience method |
| `pkg/gc/gc_test.go` | Add map GC stress test |
| `pkg/stdlib/core.go` | Add registrations for all new functions (map, math, time, random, conversion, array, string, filesystem) in `RegisterAll()` |
| `pkg/stdlib/array_ops.go` | Add 12 new array functions: array_find, array_any, array_all, array_sum, array_min, array_max, array_take, array_drop, array_chunk, array_unique, array_join, array_slice |
| `pkg/stdlib/string_ops.go` | Add 6 new string functions: string_index_of, string_repeat, string_pad_left, string_pad_right, string_bytes, string_join |
| `pkg/stdlib/math_ops.go` | Add 16 new math functions: sin, cos, tan, asin, acos, atan, atan2, log, log2, log10, exp, pi, e, gcd, lcm, clamp |
| `pkg/stdlib/io.go` | Add 8 new filesystem functions: file_exists, dir_list, dir_create, path_join, path_dirname, path_basename, path_extension, file_size |

### VM Bridge (GC ↔ VM Heap)

The file that bridges GC objects with VM heap objects (look for where
`gc.KindChannel` is used — that's where `gc.KindMap` needs to be
added for tracing map children).  This is likely in `pkg/vm/vm.go` or
a dedicated `pkg/vm/gc_bridge.go` file.

---

## 11) Task Dependency Graph

```
Wave 1 (all parallel, no dependencies):
  T1: ObjMap core (value.go, heap.go, builtins.go)
  T4: Math stdlib (math_ops.go)
  T5: Time + Random + Conversions (time_ops.go, core.go)
  T6: Array + String expansion (array_ops.go, string_ops.go)
  T7: Filesystem expansion (io.go)

Wave 2 (depends on T1):
  T2: GC integration (gc.go, write_barrier.go)
  T3: Map stdlib functions (map_ops.go, core.go)

Wave 3 (depends on T1, T2, T3, T6):
  T8: Integration tests + example programs (.ryx/.expected files)
```

**Critical path:** T1 → T3 → T8  (Map core → Map stdlib → Integration tests)

**Maximum parallelism:** 5 tasks in Wave 1 can run simultaneously.

---

## 12) Verification

```bash
# Full test suite (should pass 753+ existing tests plus all new ones)
go test ./... -v

# Per-package verification
go test ./pkg/vm/... -v          # Map heap type, value traits
go test ./pkg/gc/... -v          # GC integration, write barriers
go test ./pkg/stdlib/... -v      # All stdlib function tests

# Integration tests (includes new .ryx programs)
go test ./tests/integration/... -v

# Quick smoke test (run examples directly if binary exists)
go run . run tests/testdata/programs/map_operations.ryx
go run . run tests/testdata/programs/word_counter.ryx
go run . run tests/testdata/programs/phone_book.ryx
go run . run tests/testdata/programs/group_by.ryx
```

### Success Criteria

1. All 753+ existing tests pass (zero regressions)
2. Map unit tests pass: new, set/get, delete, contains, len, keys, values, entries, merge, filter, map
3. Math unit tests pass: all 16 functions with edge cases
4. Array unit tests pass: all 12 new functions
5. String unit tests pass: all 6 new functions
6. Time/Random/Conversion unit tests pass: all 8 functions
7. Filesystem unit tests pass: all 8 functions
8. GC stress test passes: maps are properly traced and collected
9. All 4 integration test programs produce correct output
10. `go vet ./...` reports no issues
11. No data races under `-race` flag
