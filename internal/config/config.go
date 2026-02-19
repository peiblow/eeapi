package config

type Config struct {
	Addr string
	DB   DBConfig
}

type DBConfig struct {
	DSN string
}
