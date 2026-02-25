package app

type Telegram struct {
	Token string `yaml:"token"`
}

type Instagram struct {
	Cookies   string `yaml:"cookies"`
	Cookies64 string `yaml:"cookies64"`
}

type YouTube struct {
	Cookies   string `yaml:"cookies"`
	Cookies64 string `yaml:"cookies64"`
}

type Config struct {
	Telegram  Telegram  `yaml:"telegram"`
	Instagram Instagram `yaml:"instagram"`
	YouTube   YouTube   `yaml:"youtube"`
}
