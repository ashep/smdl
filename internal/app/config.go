package app

type Telegram struct {
	Token string `yaml:"token"`
}

type Instagram struct {
	CookiesFile string `yaml:"cookies_file"`
}

type Config struct {
	Telegram  Telegram  `yaml:"telegram"`
	Instagram Instagram `yaml:"instagram"`
}
