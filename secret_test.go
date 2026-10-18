package tinyconf

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/insei/fmap/v3"
	"github.com/stretchr/testify/assert"
)

type secretConfig struct {
	User     string
	Password Secret
	Token    *Secret
	Keys     []Secret
}

// secretMockDriver returns the values of the secrets.
type secretMockDriver struct{}

func (md *secretMockDriver) GenDoc(...*Registered) string { return "" }
func (md *secretMockDriver) GetName() string              { return "secret" }
func (md *secretMockDriver) GetValue(field fmap.Field) (*Value, error) {
	switch field.GetStructPath() {
	case "Password":
		return &Value{Source: "password", Value: Secret("password value")}, nil
	case "Keys":
		return &Value{Source: "keys", Value: []Secret{"first", "second"}}, nil
	}
	return nil, ErrValueNotFound
}

func TestSecret(t *testing.T) {
	token := Secret("token value")
	conf := secretConfig{User: "user", Password: "password value", Token: &token, Keys: []Secret{"key", ""}}

	// the secrets are masked by fmt and the encoders, the length is not shown
	assert.Equal(t, "{user ****** ****** [****** ]}", fmt.Sprintf("%v", struct {
		User     string
		Password Secret
		Token    Secret
		Keys     []Secret
	}{conf.User, conf.Password, *conf.Token, conf.Keys}))
	assert.Equal(t, `tinyconf.Secret("******")`, fmt.Sprintf("%T(%#v)", conf.Password, conf.Password))
	data, err := json.Marshal(conf)
	assert.NoError(t, err)
	assert.Equal(t, `{"User":"user","Password":"******","Token":"******","Keys":["******",""]}`, string(data))

	// the value is available
	assert.Equal(t, "password value", conf.Password.Value())
	assert.Equal(t, "password value", string(conf.Password))
	assert.Equal(t, "", Secret("").String())
}

func TestSecret_Parse(t *testing.T) {
	c, err := New(WithDriver(&secretMockDriver{}))
	assert.NoError(t, err)
	conf := &secretConfig{}
	assert.NoError(t, c.Register(conf))
	assert.NoError(t, c.Parse(context.Background(), conf))
	assert.Equal(t, Secret("password value"), conf.Password)
	assert.Equal(t, []Secret{"first", "second"}, conf.Keys)
}
