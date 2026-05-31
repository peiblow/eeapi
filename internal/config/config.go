package config

type Config struct {
	Addr        string
	DB          DBConfig
	ArtifactDir string
}

type DBConfig struct {
	DSN string
}
