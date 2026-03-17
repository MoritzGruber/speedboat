package engine

type Issue struct {
	ID     string                 `json:"id"`
	Key    string                 `json:"key"`
	Fields map[string]interface{} `json:"fields"`
}

// Collector defines the interface for fetching tickets.
type Collector interface {
	List() ([]Issue, error)
}

type Connetor interface {
	List() ([]Issue, error)
	Get(id string) (Issue, error)
	Update(id string, issue Issue) (Issue, error)
}
