package tapocam

type Camera struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Host string `json:"host"`
	User string `json:"user"`
	Pass string `json:"pass"`
}

type Found struct{ Host, Model string }
