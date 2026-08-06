package connector

type Connector interface {
	Connect()
	Close()
}
