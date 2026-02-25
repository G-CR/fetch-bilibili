package creator

type Entry struct {
	UID      string `json:"uid" yaml:"uid"`
	Name     string `json:"name" yaml:"name"`
	Platform string `json:"platform" yaml:"platform"`
	Status   string `json:"status" yaml:"status"`
}
