package contract

type Device struct {
	IP           string
	MAC          string
	Hostname     string
	Manufacturer string
}

type Scanner interface {
	Scan(netFrom string, netTo string) ([]Device, error)
}
