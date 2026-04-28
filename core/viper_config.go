package core

import "github.com/spf13/viper"

type ViperConfig struct {
	v *viper.Viper
}

func NewViperConfig(v *viper.Viper) *ViperConfig {
	return &ViperConfig{v: v}
}

func (c *ViperConfig) GetString(key string) string {
	return c.v.GetString(key)
}

func (c *ViperConfig) GetInt(key string) int {
	return c.v.GetInt(key)
}

func (c *ViperConfig) GetBool(key string) bool {
	return c.v.GetBool(key)
}

func (c *ViperConfig) GetStringSlice(key string) []string {
	return c.v.GetStringSlice(key)
}

func (c *ViperConfig) Get(key string) interface{} {
	return c.v.Get(key)
}

func (c *ViperConfig) Unmarshal(key string, target interface{}) error {
	return c.v.UnmarshalKey(key, target)
}

func (c *ViperConfig) Set(key string, value interface{}) {
	c.v.Set(key, value)
}

func (c *ViperConfig) Save() error {
	return c.v.WriteConfig()
}
