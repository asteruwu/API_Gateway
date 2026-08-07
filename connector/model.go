package connector

type Connector interface {
	Connect() error
	Close()
}
