package acaia

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"github.com/influxdata/telegraf"
	"github.com/influxdata/telegraf/plugins/inputs"
	"sync"
	"time"
	"tinygo.org/x/bluetooth"
)

func init() {
	inputs.Add("acaia", func() telegraf.Input { return &AcaiaInput{} })
}

// Validate interface implementations
var (
	_ telegraf.Initializer  = &AcaiaInput{}
	_ telegraf.ServiceInput = &AcaiaInput{}
)

type AcaiaInput struct {
	Guid string `toml:"guid"`

	acc    telegraf.Accumulator
	wg     sync.WaitGroup
	device *bluetooth.Device
	weight chan float64
	Log    telegraf.Logger `toml:"-"`
}

func (*AcaiaInput) SampleConfig() string {
	return ""
}

func (s *AcaiaInput) Init() error {
	if s.Guid == "" {
		return errors.New("guid is not set")
	}

	return nil
}

func (s *AcaiaInput) Start(acc telegraf.Accumulator) error {
	adapter := bluetooth.DefaultAdapter
	err := adapter.Enable()
	if err != nil {
		return err
	}

	ch := make(chan bluetooth.ScanResult, 1)
	err = adapter.Scan(func(adapter *bluetooth.Adapter, result bluetooth.ScanResult) {
		if result.Address.String() == s.Guid {
			err = adapter.StopScan()
			if err != nil {
				return
			}
			ch <- result
		}
	})
	if err != nil {
		return err
	}

	result := <-ch
	s.device, err = adapter.Connect(result.Address, bluetooth.ConnectionParams{})
	if err != nil {
		return err
	}

	services, err := s.device.DiscoverServices([]bluetooth.UUID{bluetooth.ServiceUUIDInternetProtocolSupport})
	if err != nil {
		return err
	}
	if len(services) == 0 {
		return errors.New("could not find IP service")
	}
	service := services[0]

	chars, err := service.DiscoverCharacteristics([]bluetooth.UUID{bluetooth.New16BitUUID(0x2A80)})
	if err != nil {
		return err
	}
	if len(chars) == 0 {
		return errors.New("could not find characteristic")
	}
	char := chars[0]

	s.weight = make(chan float64)
	err = char.EnableNotifications(s.readEvent)
	if err != nil {
		return err
	}

	auth1 := []byte{
		0xef, 0xdd,
		0x0b,
		0x30, 0x31, 0x32, 0x33, 0x34, 0x35, 0x36, 0x37, 0x38, 0x39,
		0x30, 0x31, 0x32, 0x33, 0x34,
		0x9a, 0x6d,
	}

	auth2 := []byte{
		0xEF, 0xDD,
		0x0C,
		0x09, 0x00, 0x01, 0x01, 0x02, 0x02, 0x05, 0x03, 0x04,
		0x15, 0x06,
	}

	_, err = char.WriteWithoutResponse(auth1)
	if err != nil {
		return err
	}

	_, err = char.WriteWithoutResponse(auth2)
	if err != nil {
		return err
	}

	s.Log.Info("authenticated")
	//go s.ping(char)

	//select {
	//case data := <-s.weight:
	//	fields := map[string]interface{}{"value": data}
	//	acc.AddFields("weight", fields, nil)
	//}
	go func() {
		for {
			data := <-s.weight
			fields := map[string]interface{}{"value": data}
			acc.AddFields("weight", fields, nil)
		}
	}()

	return nil
}

func (s *AcaiaInput) Stop() {
	s.device.Disconnect()
}

func (s *AcaiaInput) Gather(_ telegraf.Accumulator) error {
	return nil
}

func (s *AcaiaInput) ping(char bluetooth.DeviceCharacteristic) {
	for {
		s.Log.Info("ping...")
		char.WriteWithoutResponse(
			[]byte{
				0xEF, 0xDD,
				0x00,
				0x02, 0x00,
				0x02, 0x00,
			},
		)
		time.Sleep(5 * time.Second)
	}
}

func (s *AcaiaInput) readEvent(buf []byte) {
	//s.Log.Info("read event")
	if !bytes.HasPrefix(buf, []byte{0x08, 0x05}) {
		//fmt.Printf("%x\n", buf)
		return
	}

	//fmt.Printf("%x\n", buf)
	var result int32

	err := binary.Read(bytes.NewBuffer(buf[2:6]), binary.LittleEndian, &result)
	if err != nil {
		fmt.Println(err)
	}
	weight := float64(result) / 100.0
	s.Log.Info("weight: ", weight)
	s.weight <- weight
}
