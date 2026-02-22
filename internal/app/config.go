package app

type Telegram struct {
	Token string `yaml:"token"`
}

type Instagram struct {
	Cookies string `yaml:"cookies"`
}

type Config struct {
	Telegram  Telegram  `yaml:"telegram"`
	Instagram Instagram `yaml:"instagram"`
}
