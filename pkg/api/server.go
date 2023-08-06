package api

type Server interface {
	Name() string
	Run() error
	Transfer() (bool, string)
	Restore(data string)
}
