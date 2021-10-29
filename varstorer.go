package main

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"reflect"

	bosherr "github.com/cloudfoundry/bosh-utils/errors"
	boshlog "github.com/cloudfoundry/bosh-utils/logger"
	boshsys "github.com/cloudfoundry/bosh-utils/system"
	cfgtypes "github.com/cloudfoundry/config-server/types"
	"github.com/mitchellh/mapstructure"
	"gopkg.in/yaml.v2"

	boshtpl "github.com/cloudfoundry/bosh-cli/director/template"
)

type VarsFSStore struct {
	FS boshsys.FileSystem

	ValueGeneratorFactory cfgtypes.ValueGeneratorFactory

	path string

	statics boshtpl.StaticVariables
}

var _ boshtpl.Variables = VarsFSStore{}

func NewVarsFSStore(path string) *VarsFSStore {
	return &VarsFSStore{
		FS:                    boshsys.NewOsFileSystemWithStrictTempRoot(boshlog.NewLogger(boshlog.LevelNone)),
		ValueGeneratorFactory: cfgtypes.NewValueGeneratorConcrete(nil),
		path:                  path,
		statics:               map[string]interface{}{},
	}
}

func (s VarsFSStore) LoadAndStore(varsDefinitions []boshtpl.VariableDefinition) error {
	for _, def := range varsDefinitions {
		_, _, err := s.Get(def)
		if err != nil {
			return err
		}
	}

	vars, err := s.load()
	if err != nil {
		return err
	}
	return s.save(vars)
}

func (s VarsFSStore) IsSet() bool { return len(s.path) > 0 }

func (s VarsFSStore) Get(varDef boshtpl.VariableDefinition) (interface{}, bool, error) {
	vars, err := s.load()
	if err != nil {
		return nil, false, err
	}

	val, found := vars[varDef.Name]
	if found {
		return val, true, nil
	}

	if len(varDef.Type) == 0 {
		return nil, false, nil
	}

	val, err = s.generateAndSet(varDef)
	if err != nil {
		return nil, false, bosherr.WrapErrorf(err, "Generating variable '%s'", varDef.Name)
	}

	return val, true, nil
}

func (s VarsFSStore) List() ([]boshtpl.VariableDefinition, error) {
	vars, err := s.load()
	if err != nil {
		return nil, err
	}

	return vars.List()
}

func (s VarsFSStore) generateAndSet(varDef boshtpl.VariableDefinition) (interface{}, error) {
	optionBase64 := struct {
		Base64 bool `mapstructure:"base64"`
	}{}
	if opts, ok := varDef.Options.(map[interface{}]interface{}); ok {
		err := mapstructure.Decode(varDef.Options, &optionBase64)
		if err != nil {
			return nil, err
		}
		delete(opts, "base64")
		varDef.Options = opts
	}

	generator, err := s.ValueGeneratorFactory.GetGenerator(varDef.Type)
	if err != nil {
		return nil, err
	}

	val, err := generator.Generate(varDef.Options)
	if err != nil {
		return nil, err
	}

	if optionBase64.Base64 {
		val, err = s.b64Value(val)
		if err != nil {
			return nil, err
		}
	}

	err = s.set(varDef.Name, val)
	if err != nil {
		return nil, err
	}

	return val, nil
}

func (s VarsFSStore) b64Value(val interface{}) (interface{}, error) {
	if reflect.TypeOf(val).Kind() == reflect.Struct {
		result, err := yaml.Marshal(val)
		if err != nil {
			return nil, err
		}
		newVal := map[interface{}]interface{}{}
		err = yaml.Unmarshal(result, &newVal)
		if err != nil {
			return nil, err
		}
		val = newVal
	}
	switch v := val.(type) {
	case []interface{}:
		for i, newV := range v {
			v[i], _ = s.b64Value(newV)
		}
		return v, nil
	case map[interface{}]interface{}:
		for newK, newV := range v {
			v[newK], _ = s.b64Value(newV)
		}
		return v, nil
	case string:
		return base64.StdEncoding.EncodeToString([]byte(v)), nil
	}
	return base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%v", val))), nil
}

func (s VarsFSStore) set(key string, val interface{}) error {
	vars, err := s.load()
	if err != nil {
		return err
	}

	vars[key] = val

	return s.save(vars)
}

func (s VarsFSStore) load() (boshtpl.StaticVariables, error) {
	if s.FS == nil {
		s.FS = boshsys.NewOsFileSystemWithStrictTempRoot(boshlog.NewLogger(boshlog.LevelNone))
	}

	vars := s.statics

	if s.FS.FileExists(s.path) {
		bytes, err := s.FS.ReadFile(s.path)
		if err != nil {
			return vars, err
		}

		err = yaml.Unmarshal(bytes, &vars)
		if err != nil {
			return vars, bosherr.WrapErrorf(err, "Deserializing variables file store '%s'", s.path)
		}
	}
	if vars == nil {
		return boshtpl.StaticVariables{}, nil
	}

	return vars, nil
}

func (s VarsFSStore) save(vars boshtpl.StaticVariables) error {
	if s.FS == nil {
		s.FS = boshsys.NewOsFileSystemWithStrictTempRoot(boshlog.NewLogger(boshlog.LevelNone))
	}

	bytes, err := yaml.Marshal(vars)
	if err != nil {
		return bosherr.WrapErrorf(err, "Serializing variables")
	}

	err = s.FS.WriteFile(s.path, bytes)
	if err != nil {
		return bosherr.WrapErrorf(err, "Writing variables to file store '%s'", s.path)
	}

	return nil
}

type VarsCertLoader struct {
	vars boshtpl.Variables
}

func NewVarsCertLoader(vars boshtpl.Variables) VarsCertLoader {
	return VarsCertLoader{vars}
}

func (l VarsCertLoader) LoadCerts(name string) (*x509.Certificate, *rsa.PrivateKey, error) {
	val, found, err := l.vars.Get(boshtpl.VariableDefinition{Name: name})
	if err != nil {
		return nil, nil, err
	} else if !found {
		return nil, nil, fmt.Errorf("Expected to find variable '%s' with a certificate", name)
	}

	// Convert to YAML for easier struct parsing
	valBytes, err := yaml.Marshal(val)
	if err != nil {
		return nil, nil, bosherr.WrapErrorf(err, "Expected variable '%s' to be serializable", name)
	}

	type CertVal struct {
		Certificate string
		PrivateKey  string `yaml:"private_key"`
	}

	var certVal CertVal

	err = yaml.Unmarshal(valBytes, &certVal)
	if err != nil {
		return nil, nil, bosherr.WrapErrorf(err, "Expected variable '%s' to be deserializable", name)
	}

	crt, err := l.parseCertificate(certVal.Certificate)
	if err != nil {
		return nil, nil, err
	}

	key, err := l.parsePrivateKey(certVal.PrivateKey)
	if err != nil {
		return nil, nil, err
	}

	return crt, key, nil
}

func (VarsCertLoader) parseCertificate(data string) (*x509.Certificate, error) {
	fromB64, err := base64.StdEncoding.DecodeString(data)
	if err == nil {
		data = string(fromB64)
	}
	cpb, _ := pem.Decode([]byte(data))
	if cpb == nil {
		return nil, bosherr.Error("Certificate did not contain PEM formatted block")
	}

	crt, err := x509.ParseCertificate(cpb.Bytes)
	if err != nil {
		return nil, bosherr.WrapError(err, "Parsing certificate")
	}

	return crt, nil
}

func (VarsCertLoader) parsePrivateKey(data string) (*rsa.PrivateKey, error) {
	fromB64, err := base64.StdEncoding.DecodeString(data)
	if err == nil {
		data = string(fromB64)
	}
	kpb, _ := pem.Decode([]byte(data))
	if kpb == nil {
		return nil, bosherr.Error("Private key did not contain PEM formatted block")
	}

	key, err := x509.ParsePKCS1PrivateKey(kpb.Bytes)
	if err != nil {
		return nil, bosherr.WrapError(err, "Parsing private key")
	}

	return key, nil
}
