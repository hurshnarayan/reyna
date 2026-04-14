package qdrant

// Collection names — shared constants so every caller uses the same strings.
const (
	CollectionFiles    = "reyna_files"
	CollectionMemories = "reyna_memories"
)

// FilePointID builds a deterministic ID for a file embedding so re-embedding
// the same file overwrites rather than duplicates.
func FilePointID(fileID int64) int64 { return fileID }

// MemoryPointID builds an ID for a memory chunk. Multiple chunks per memory
// are disambiguated by chunkIndex.
// memoryID * 1000 + chunkIndex supports up to 1000 chunks per memory, plenty
// for syllabus-sized uploads (~1.5M runes worth).
func MemoryPointID(memoryID int64, chunkIndex int) int64 {
	return memoryID*1000 + int64(chunkIndex)
}

// MemoryIDsFromBase returns the range of point IDs belonging to a memory,
// used when deleting all chunks of a memory.
func MemoryIDRange(memoryID int64) (int64, int64) {
	return memoryID * 1000, memoryID*1000 + 999
}
