package repository

type Node struct {
	ID       string
	Label    string
	Kind     string
	Size     int
	Children int
	Depth    int
}

type Edge struct {
	Source string
	Target string
	Type   string
	Types  []string
	Count  int
}
