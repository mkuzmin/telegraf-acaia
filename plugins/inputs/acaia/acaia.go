package acaia

import (
	"github.com/influxdata/telegraf"
	"github.com/influxdata/telegraf/plugins/inputs"
)

var (
	_ telegraf.Initializer = &AcaiaInput{}
	_ telegraf.Input       = &AcaiaInput{}
)

type AcaiaInput struct {
	//Ok  bool            `toml:"ok"`
	//Log telegraf.Logger `toml:"-"`
}

func (*AcaiaInput) SampleConfig() string {
	return ""
}

func (s *AcaiaInput) Init() error {
	return nil
}

func (s *AcaiaInput) Gather(acc telegraf.Accumulator) error {
	acc.AddFields("weight", map[string]interface{}{"value": 1.0}, nil)

	return nil
}

func init() {
	inputs.Add("acaia", func() telegraf.Input { return &AcaiaInput{} })
}
