package asyncapi

import (
	"bytes"
	"errors"
	"fmt"
	"strconv"

	"github.com/asyncapi/parser-go/pkg/parser"
	"github.com/buger/jsonparser"
	"github.com/iancoleman/strcase"
)

const (
	ChannelsKey        = "channels"
	SubscribeKey       = "subscribe"
	MessageKey         = "message"
	PayloadKey         = "payload"
	PropertiesKey      = "properties"
	EnumKey            = "enum"
	ServersKey         = "servers"
	URLKey             = "url"
	ProtocolKey        = "protocol"
	ProtocolVersionKey = "protocolVersion"
	DescriptionKey     = "description"
	NameKey            = "name"
	TitleKey           = "title"
	SummaryKey         = "summary"
	TypeKey            = "type"
	FormatKey          = "format"
	MinimumKey         = "minimum"
	MaximumKey         = "maximum"
	OperationIDKey     = "operationId"
	SecurityKey        = "security"
	BindingsKey        = "bindings"
	KafkaKey           = "kafka"
	TraitsKey          = "traits"
	ParametersKey      = "parameters"
	SchemaKey          = "schema"
)

type AsyncAPI struct {
	Channels map[string]*ChannelItem
	Servers  map[string]*Server
}

type SecurityRequirement struct {
	Requirements map[string][]string
}

type Binding struct {
	Value     []byte
	ValueType jsonparser.ValueType
}

// Server object is defined here:
// https://www.asyncapi.com/docs/reference/specification/v2.4.0#serverObject
type Server struct {
	URL             string
	Protocol        string
	ProtocolVersion string
	Description     string
	Security        []*SecurityRequirement
	Bindings        map[string]map[string]*Binding
}

// OperationTrait object is defined here:
// https://www.asyncapi.com/docs/reference/specification/v2.4.0#operationTraitObject
type OperationTrait struct {
	Bindings map[string]map[string]*Binding
}

// ChannelItem object is defined here:
// https://www.asyncapi.com/docs/reference/specification/v2.4.0#channelItemObject
type ChannelItem struct {
	Message     *Message
	Parameters  map[string]string
	OperationID string
	Traits      []*OperationTrait
	Servers     []string
}

type Enum struct {
	Value     []byte
	ValueType jsonparser.ValueType
}

// Property object is derived from Schema object.
// https://www.asyncapi.com/docs/reference/specification/v2.4.0#schemaObject
type Property struct {
	Description string
	Minimum     int
	Maximum     int
	Type        string
	Format      string
	Enum        []*Enum
}

// Payload is definition of the message payload. It can be of any type but defaults to Schema object.
// It must match the schema format, including encoding type - e.g Avro should be inlined as
// either a YAML or JSON object NOT a string to be parsed as YAML or JSON.
type Payload struct {
	Type       string
	Properties map[string]*Property
}

// Message object is defined here:
// https://www.asyncapi.com/docs/reference/specification/v2.4.0#messageObject
type Message struct {
	Name        string
	Summary     string
	Title       string
	Description string
	Payload     *Payload
}

type walker struct {
	document *bytes.Buffer
	asyncapi *AsyncAPI
}

func extractStringArray(key string, data []byte) ([]string, error) {
	var result []string
	_, err := jsonparser.ArrayEach(data, func(value []byte, dataType jsonparser.ValueType, _ int, _ error) {
		result = append(result, string(value))
	}, key)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func extractString(key string, data []byte) (string, error) {
	value, dataType, _, err := jsonparser.Get(data, key)
	if err != nil {
		return "", err
	}
	if dataType != jsonparser.String {
		return "", fmt.Errorf("key: %s has to be a string", key)
	}
	return string(value), nil
}

func extractInteger(key string, data []byte) (int, error) {
	value, dataType, _, err := jsonparser.Get(data, key)
	if err != nil {
		return 0, err
	}
	if dataType != jsonparser.Number {
		return 0, fmt.Errorf("key: %s has to be a number", key)
	}
	return strconv.Atoi(string(value))
}

func (w *walker) enterPropertyObject(channel, key, data []byte) error {
	property := &Property{}
	// Not mandatory
	description, err := extractString(DescriptionKey, data)
	if err == nil {
		property.Description = description
	}

	// Not mandatory
	format, err := extractString(FormatKey, data)
	if err == nil {
		property.Format = format
	}

	// Mandatory
	tpe, err := extractString(TypeKey, data)
	if err == jsonparser.KeyPathNotFoundError {
		return fmt.Errorf("property: %s is required in %s, channel: %s", TypeKey, key, channel)
	}
	if err != nil {
		return err
	}
	property.Type = tpe

	// Not mandatory
	minimum, err := extractInteger(MinimumKey, data)
	if err == nil {
		property.Minimum = minimum
	}

	// Not mandatory
	maximum, err := extractInteger(MaximumKey, data)
	if err == nil {
		property.Maximum = maximum
	}

	// Not mandatory
	_, err = jsonparser.ArrayEach(data, func(enumValue []byte, dataType jsonparser.ValueType, _ int, err error) {
		property.Enum = append(property.Enum, &Enum{
			Value:     enumValue,
			ValueType: dataType,
		})
	}, EnumKey)
	if errors.Is(err, jsonparser.KeyPathNotFoundError) {
		err = nil
	}
	if err != nil {
		return err
	}

	channelItem, ok := w.asyncapi.Channels[string(channel)]
	if !ok {
		return fmt.Errorf("channel: %s is missing", channel)
	}
	// Field names should use camelCase. Many GraphQL clients are written in JavaScript, Java, Kotlin, or Swift,
	// all of which recommend camelCase for variable names.
	channelItem.Message.Payload.Properties[strcase.ToLowerCamel(string(key))] = property
	return nil
}

func (w *walker) enterPropertiesObject(channel, data []byte) error {
	propertiesValue, dataType, _, err := jsonparser.Get(data, PropertiesKey)
	if errors.Is(err, jsonparser.KeyPathNotFoundError) {
		return fmt.Errorf("key: %s is missing", PropertiesKey)
	}
	if dataType != jsonparser.Object {
		return fmt.Errorf("key: %s has to be a JSON object", propertiesValue)
	}

	return jsonparser.ObjectEach(propertiesValue, func(key []byte, value []byte, dataType jsonparser.ValueType, _ int) error {
		return w.enterPropertyObject(channel, key, value)
	})
}

func (w *walker) enterPayloadObject(key, data []byte) error {
	payload, dataType, _, err := jsonparser.Get(data, PayloadKey)
	if errors.Is(err, jsonparser.KeyPathNotFoundError) {
		return fmt.Errorf("key: %s is missing", PayloadKey)
	}
	if dataType != jsonparser.Object {
		return fmt.Errorf("key: %s has to be a JSON object", PayloadKey)
	}

	p := &Payload{Properties: make(map[string]*Property)}
	typeValue, err := extractString(TypeKey, payload)
	if err == nil {
		p.Type = typeValue
	}

	channel, ok := w.asyncapi.Channels[string(key)]
	if !ok {
		return fmt.Errorf("channel: %s is missing", key)
	}
	channel.Message.Payload = p
	return w.enterPropertiesObject(key, payload)
}

func (w *walker) enterMessageObject(channelName, data []byte) error {
	msg := &Message{}
	name, err := extractString(NameKey, data)
	if err == jsonparser.KeyPathNotFoundError {
		name = string(channelName)
		err = nil
	}
	if err != nil {
		return err
	}
	msg.Name = name

	summary, err := extractString(SummaryKey, data)
	if err == nil {
		msg.Summary = summary
	}

	title, err := extractString(TitleKey, data)
	if err == nil {
		msg.Title = title
	}

	description, err := extractString(DescriptionKey, data)
	if err == nil {
		msg.Description = description
	}
	channel, ok := w.asyncapi.Channels[string(channelName)]
	if !ok {
		return fmt.Errorf("channel: %s is missing", channelName)
	}
	channel.Message = msg
	return w.enterPayloadObject(channelName, data)
}

func (w *walker) enterOperationTraitsObject(channelName []byte, data []byte) error {
	// Not Mandatory
	traitsValue, dataType, _, err := jsonparser.Get(data, TraitsKey)
	if errors.Is(err, jsonparser.KeyPathNotFoundError) {
		return nil
	}
	if dataType != jsonparser.Array {
		return errors.New("traits has to be an array")
	}
	opt := &OperationTrait{
		Bindings: make(map[string]map[string]*Binding),
	}

	var bindingValues [][]byte
	_, err = jsonparser.ArrayEach(traitsValue, func(bindingValue []byte, dataType jsonparser.ValueType, offset int, err error) {
		bindingValues = append(bindingValues, bindingValue)
	})
	if err != nil {
		return err
	}

	for _, bindingValue := range bindingValues {
		kafkaValue, _, _, err := jsonparser.Get(bindingValue, BindingsKey, KafkaKey)
		if errors.Is(err, jsonparser.KeyPathNotFoundError) {
			return nil
		}

		err = jsonparser.ObjectEach(kafkaValue, func(key []byte, kafkaBindingItemValue []byte, dataType jsonparser.ValueType, _ int) error {
			if dataType != jsonparser.String {
				// Currently, we only support String values.
				return nil
			}
			b := &Binding{
				Value:     kafkaBindingItemValue,
				ValueType: dataType,
			}
			_, ok := opt.Bindings[KafkaKey]
			if !ok {
				opt.Bindings[KafkaKey] = make(map[string]*Binding)
			}
			opt.Bindings[KafkaKey][string(key)] = b
			return nil
		})
		if err != nil {
			return err
		}
	}

	channel, ok := w.asyncapi.Channels[string(channelName)]
	if !ok {
		return fmt.Errorf("channel: %s is missing", channelName)
	}
	channel.Traits = append(channel.Traits, opt)
	return nil
}

func (w *walker) enterParametersObject(channelItem *ChannelItem, data []byte) error {
	// Not mandatory
	parametersValue, _, _, err := jsonparser.Get(data, ParametersKey)
	if err == jsonparser.KeyPathNotFoundError {
		return nil
	}
	if err != nil {
		return err
	}
	return jsonparser.ObjectEach(parametersValue, func(parameterName []byte, parameterValue []byte, _ jsonparser.ValueType, _ int) error {
		parameterType, _, _, perr := jsonparser.Get(parameterValue, SchemaKey, TypeKey)
		if perr != nil {
			return perr
		}
		channelItem.Parameters[string(parameterName)] = string(parameterType)
		return nil
	})
}

func (w *walker) enterChannelItemObject(channelName []byte, data []byte) error {
	subscribeValue, dataType, _, err := jsonparser.Get(data, SubscribeKey)
	if errors.Is(err, jsonparser.KeyPathNotFoundError) {
		return nil
	}
	if err != nil {
		return err
	}

	if dataType != jsonparser.Object {
		return fmt.Errorf("%s has to be a JSON object", SubscribeKey)
	}

	messageValue, dataType, _, err := jsonparser.Get(subscribeValue, MessageKey)
	if errors.Is(err, jsonparser.KeyPathNotFoundError) {
		return fmt.Errorf("message item is missing in channel: %s", channelName)
	}
	if err != nil {
		return err
	}

	if dataType != jsonparser.Object {
		return fmt.Errorf("%s has to be a JSON object", MessageKey)
	}

	operationID, err := extractString(OperationIDKey, subscribeValue)
	if errors.Is(err, jsonparser.KeyPathNotFoundError) {
		return fmt.Errorf("key: %s is required in channel: %s", OperationIDKey, channelName)
	}
	if err != nil {
		return err
	}

	// Not mandatory
	servers, err := extractStringArray(ServersKey, data)
	if errors.Is(err, jsonparser.KeyPathNotFoundError) {
		err = nil
	}
	if err != nil {
		return err
	}

	channelItem := &ChannelItem{
		OperationID: operationID,
		Servers:     servers,
		Parameters:  make(map[string]string),
	}

	err = w.enterParametersObject(channelItem, data)
	if err != nil {
		return err
	}

	w.asyncapi.Channels[string(channelName)] = channelItem

	err = w.enterOperationTraitsObject(channelName, subscribeValue)
	if err != nil {
		return err
	}

	return w.enterMessageObject(channelName, messageValue)
}

func (w *walker) enterChannelObject() error {
	value, dataType, _, err := jsonparser.Get(w.document.Bytes(), ChannelsKey)
	if err == jsonparser.KeyPathNotFoundError {
		return fmt.Errorf("key: %s is missing", ChannelsKey)
	}
	if err != nil {
		return err
	}

	if dataType != jsonparser.Object {
		return fmt.Errorf("%s has to be a JSON object", ChannelsKey)
	}

	return jsonparser.ObjectEach(value, func(key []byte, value []byte, dataType jsonparser.ValueType, offset int) error {
		if dataType != jsonparser.Object {
			return fmt.Errorf("%s has to be a JSON object", key)
		}
		err = w.enterChannelItemObject(key, value)
		if err != nil {
			return err
		}
		return nil
	})
}

func (w *walker) enterSecurityRequirementObject(key, data []byte, s *Server) error {
	sr := &SecurityRequirement{Requirements: make(map[string][]string)}

	_, err := jsonparser.ArrayEach(data, func(value3 []byte, dataType2 jsonparser.ValueType, _ int, _ error) {
		sr.Requirements[string(key)] = append(sr.Requirements[string(key)], string(value3))
	})
	if err != nil {
		return err
	}

	if len(sr.Requirements) > 0 {
		s.Security = append(s.Security, sr)
	}
	return nil
}

func (w *walker) enterSecurityObject(s *Server, data []byte) error {
	// Not mandatory
	var securityObjectItems [][]byte
	_, err := jsonparser.ArrayEach(data, func(securityObjectItem []byte, dataType jsonparser.ValueType, _ int, err error) {
		securityObjectItems = append(securityObjectItems, securityObjectItem)
	}, SecurityKey)
	if err == jsonparser.KeyPathNotFoundError {
		return nil
	}
	if err != nil {
		return err
	}

	for _, securityObjectItem := range securityObjectItems {
		err = jsonparser.ObjectEach(securityObjectItem, func(key []byte, value []byte, _ jsonparser.ValueType, _ int) error {
			return w.enterSecurityRequirementObject(key, value, s)
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func (w *walker) enterServerBindingsObject(s *Server, data []byte) error {
	// Not mandatory
	kafkaValue, _, _, err := jsonparser.Get(data, BindingsKey, KafkaKey)
	if errors.Is(err, jsonparser.KeyPathNotFoundError) {
		return nil
	}
	if err != nil {
		return err
	}

	return jsonparser.ObjectEach(kafkaValue, func(key []byte, kafkaBindingItemValue []byte, dataType jsonparser.ValueType, _ int) error {
		if dataType != jsonparser.String {
			return nil
		}
		b := &Binding{
			Value:     kafkaBindingItemValue,
			ValueType: dataType,
		}
		_, ok := s.Bindings[KafkaKey]
		if !ok {
			s.Bindings[KafkaKey] = make(map[string]*Binding)
		}
		s.Bindings[KafkaKey][string(key)] = b
		return nil
	})
}

func (w *walker) enterServerObject(key, data []byte) error {
	s := &Server{
		Bindings: map[string]map[string]*Binding{},
	}

	// Mandatory
	urlValue, err := extractString(URLKey, data)
	if err != nil {
		return err
	}
	s.URL = urlValue

	protocolValue, err := extractString(ProtocolKey, data)
	if err != nil {
		return err
	}
	s.Protocol = protocolValue

	// Not mandatory
	protocolVersionValue, err := extractString(ProtocolVersionKey, data)
	if err == nil {
		s.ProtocolVersion = protocolVersionValue
	}
	descriptionValue, err := extractString(DescriptionKey, data)
	if err == nil {
		s.Description = descriptionValue
	}

	err = w.enterSecurityObject(s, data)
	if err != nil {
		return err
	}

	err = w.enterServerBindingsObject(s, data)
	if err != nil {
		return err
	}

	w.asyncapi.Servers[string(key)] = s
	return nil
}

func (w *walker) enterServersObject() error {
	// Not Mandatory
	serverValue, dataType, _, err := jsonparser.Get(w.document.Bytes(), ServersKey)
	if err == jsonparser.KeyPathNotFoundError {
		return nil
	}
	if err != nil {
		return err
	}
	if dataType != jsonparser.Object {
		return fmt.Errorf("%s has to be a JSON object", ServersKey)
	}
	return jsonparser.ObjectEach(serverValue, func(key []byte, value []byte, dataType jsonparser.ValueType, offset int) error {
		return w.enterServerObject(key, value)
	})
}

func ParseAsyncAPIDocument(input []byte) (*AsyncAPI, error) {
	r := bytes.NewBuffer(input)
	asyncAPIParser, err := parser.New()
	if err != nil {
		return nil, err
	}

	buf := bytes.NewBuffer(nil)
	err = asyncAPIParser(r, buf)
	if err != nil {
		return nil, err
	}

	w := &walker{
		document: buf,
		asyncapi: &AsyncAPI{
			Channels: make(map[string]*ChannelItem),
			Servers:  make(map[string]*Server),
		},
	}

	err = w.enterChannelObject()
	if err != nil {
		return nil, err
	}

	err = w.enterServersObject()
	if err != nil {
		return nil, err
	}

	return w.asyncapi, nil
}
