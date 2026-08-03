package app

type Telegram struct {
	Token string   `yaml:"token"`
	Users []string `yaml:"users"`
}

type Instagram struct {
	Cookies string `yaml:"cookies"`
}

type YouTube struct {
	Cookies string `yaml:"cookies"`
}

type Facebook struct {
	Cookies string `yaml:"cookies"`
}

type Config struct {
	Telegram  Telegram  `yaml:"telegram"`
	Instagram Instagram `yaml:"instagram"`
	YouTube   YouTube   `yaml:"youtube"`
	Facebook  Facebook  `yaml:"facebook"`
	Proxy     string    `yaml:"proxy"`
}
