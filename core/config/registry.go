package config

var modules []ModuleSetter

func Register(m ModuleSetter) {
	modules = append(modules, m)
}

func Get(name string) ModuleSetter {
	for _, m := range modules {
		if m.Name() == name {
			return m
		}
	}
	return nil
}

func All() []ModuleSetter {
	return modules
}
