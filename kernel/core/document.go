package core

type Document struct {
	Feature  string    `json:"feature"`
	Format   string    `json:"format"`
	Version  string    `json:"version"`
	Encoding string    `json:"encoding"`
	Units    []Unit    `json:"units"`
	Sections []Section `json:"sections"`
	Meta     Meta      `json:"meta"`
}

type Unit struct {
	ID      string `json:"id"`
	Kind    string `json:"kind"`
	Speaker string `json:"speaker,omitempty"`
	Text    string `json:"text"`
	Name    string `json:"name,omitempty"`
	Prefix  string `json:"prefix,omitempty"`
	Suffix  string `json:"suffix,omitempty"`
}

type Section struct {
	Kind     string   `json:"kind"`
	Name     string   `json:"name,omitempty"`
	Offset   int      `json:"offset"`
	Size     int      `json:"size,omitempty"`
	Base64   string   `json:"base64,omitempty"`
	Encoding string   `json:"encoding,omitempty"`
	UnitSize int      `json:"unitSize,omitempty"`
	EOL      string   `json:"eol,omitempty"`
	IDs      []string `json:"ids,omitempty"`
	Values   []int32  `json:"values,omitempty"`
	Trailing string   `json:"trailing,omitempty"`
}

type Meta struct {
	Values map[string]string   `json:"values,omitempty"`
	Lists  map[string][]string `json:"lists,omitempty"`
}
