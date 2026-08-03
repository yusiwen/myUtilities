package config

type ModuleSetter interface {
	Name() string
	Set(args []string) error
}
