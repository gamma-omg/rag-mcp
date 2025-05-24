package docstore

type Doc struct {
	File   string
	Crc    uint32
	Chunks []string
}

type SearchResult struct {
	Text  string
	File  string
	Score float32
}

type IngestedDoc struct {
	File string
	Crc  uint32
}
