package acaia

import (
	"encoding/binary"
	"errors"
	"github.com/influxdata/telegraf"
	"github.com/influxdata/telegraf/plugins/inputs"
	"strings"
	"sync"
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
	Model string `toml:"model"`

	acc        telegraf.Accumulator
	wg         sync.WaitGroup
	device     *bluetooth.Device
	byteChan   chan byte
	weightChan chan float64
	Log        telegraf.Logger `toml:"-"`
}

func (*AcaiaInput) SampleConfig() string {
	return ""
}

func (s *AcaiaInput) Init() error {
	if s.Model == "" {
		return errors.New("model is not set")
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
		if strings.HasPrefix(result.AdvertisementPayload.LocalName(), s.Model) {
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

	var readChar, writeChar bluetooth.DeviceCharacteristic
	switch s.Model {
	case "ACAIAL1":
		serviceUUID := bluetooth.ServiceUUIDInternetProtocolSupport
		readCharUUID := bluetooth.New16BitUUID(0x2A80)
		services, err := s.device.DiscoverServices([]bluetooth.UUID{serviceUUID})
		if err != nil {
			return err
		}
		if len(services) != 1 {
			return errors.New("could not find IP service")
		}
		service := services[0]
		s.Log.Debug("service: ", service.UUID().String())

		chars, err := service.DiscoverCharacteristics([]bluetooth.UUID{readCharUUID})
		if err != nil {
			return err
		}
		if len(chars) != 1 {
			return errors.New("could not find characteristic")
		}
		readChar = chars[0]
		writeChar = readChar

	case "PEARLS":
		serviceUUID, _ := bluetooth.ParseUUID("49535343-fe7d-4ae5-8fa9-9fafd205e455")
		readCharUUID, _ := bluetooth.ParseUUID("49535343-1e4d-4bd9-ba61-23c647249616")
		writeCharUUID, _ := bluetooth.ParseUUID("49535343-8841-43f4-a8d4-ecbe34729bb3")
		services, err := s.device.DiscoverServices([]bluetooth.UUID{serviceUUID})
		if err != nil {
			return err
		}
		if len(services) != 1 {
			return errors.New("could not find IP service")
		}
		service := services[0]
		s.Log.Debug("service: ", service.UUID().String())

		chars, err := service.DiscoverCharacteristics([]bluetooth.UUID{readCharUUID, writeCharUUID})
		if err != nil {
			return err
		}
		if len(chars) != 2 {
			return errors.New("could not find characteristics")
		}
		readChar = chars[0]
		writeChar = chars[1]
	}
	s.Log.Debug("read char: ", readChar.UUID().String())
	s.Log.Debug("write char: ", writeChar.UUID().String())

	s.byteChan = make(chan byte)
	s.weightChan = make(chan float64)
	go s.processBytes()
	err = readChar.EnableNotifications(s.readEvent)
	if err != nil {
		return err
	}

	auth := []byte{
		0xef, 0xdd, // prefix
		0x0b, // message type
		0x2D, 0x2D, 0x2D, 0x2D, 0x2D,
		0x2D, 0x2D, 0x2D, 0x2D, 0x2D,
		0x2D, 0x2D, 0x2D, 0x2D, 0x2D,
		0x68, 0x3B, // checksum
	}

	config := []byte{
		0xEF, 0xDD, // prefix
		0x0C,       // message type
		0x09,       // length
		0x00, 0x01, // payload weight
		0x01, 0x02, // payload battery
		0x02, 0x05, // payload timer
		0x03, 0x04, // payload key (not used)
		0x15, 0x06, // checksum
	}

	_, err = writeChar.WriteWithoutResponse(auth)
	if err != nil {
		return err
	}

	_, err = writeChar.WriteWithoutResponse(config)
	if err != nil {
		return err
	}

	s.Log.Debug("authenticated")
	//go s.ping(readChar)

	go func() {
		for {
			data := <-s.weightChan
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

//func (s *AcaiaInput) ping(char bluetooth.DeviceCharacteristic) {
//	for {
//		s.Log.Info("ping...")
//		char.WriteWithoutResponse(
//			[]byte{
//				0xEF, 0xDD,
//				0x00,
//				0x02, 0x00,
//				0x02, 0x00,
//			},
//		)
//		time.Sleep(5 * time.Second)
//	}
//}

func (s *AcaiaInput) readEvent(buf []byte) {
	s.Log.Debugf("event: %x", buf)
	for _, b := range buf {
		s.byteChan <- b
	}
}

func (s *AcaiaInput) processBytes() {
	s.Log.Debug("process bytes")
	for {
		b := <-s.byteChan
		s.Log.Debugf("byte: %x", b)
		if b != 0xEF {
			continue
		}
		b = <-s.byteChan
		s.Log.Debugf("byte: %x", b)
		if b != 0xDD {
			continue
		}
		b = <-s.byteChan
		s.Log.Debugf("byte: %x", b)
		if b != 0x0C {
			continue
		}

		b = <-s.byteChan
		s.Log.Debugf("byte: %x", b)
		payload := []byte{b}
		size := int(payload[0])
		for i := 0; i < size-1; i++ {
			b = <-s.byteChan
			s.Log.Debugf("byte: %x", b)
			payload = append(payload, b)
		}
		s.Log.Debugf("payload: %x", payload)

		_ = <-s.byteChan // crc1
		_ = <-s.byteChan // crc2
		// TODO: check crc

		if size != 8 {
			continue
		}

		result := binary.LittleEndian.Uint16(payload[2:4])

		var precision float64
		switch payload[6] {
		case 0x01:
			precision = 10.0
		case 0x02:
			precision = 100.0
		}

		var negative float64
		if payload[7]&0x02 == 0 {
			negative = 1.0
		} else {
			negative = -1.0
		}

		s.weightChan <- negative * float64(result) / precision
	}
}
