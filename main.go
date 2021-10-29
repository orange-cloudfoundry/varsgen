package main

import (
	"fmt"
	"io/ioutil"
	"os"

	boshtpl "github.com/cloudfoundry/bosh-cli/director/template"
	cfgtypes "github.com/cloudfoundry/config-server/types"
	"github.com/jessevdk/go-flags"
	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"
)

type Options struct {
	Verbose        bool   `short:"v" long:"verbose" description:"Set to pass in verbose mode."`
	Version        func() `          long:"version" description:"Show version"`
	DefinitionFile string `short:"d" long:"def-file" required:"true" description:"path a file containing definition of credentials"`
	StoreFile      string `short:"s" long:"var-store" positional-args:"true" required:"true" description:"path to a file which will contains generated credentials"`
}

type VersionInfo struct {
	Version string
	Commit  string
	Date    string
}

var version string
var commit string
var date string

var mainOpts = &Options{}

var parser = flags.NewParser(mainOpts, flags.HelpFlag|flags.PassDoubleDash|flags.IgnoreUnknown)

func Parse(vInfo VersionInfo, args []string) error {

	askVersion := false

	mainOpts.Version = func() {
		askVersion = true
		fmt.Printf("gotof %v, commit %v, built at %v\n", vInfo.Version, vInfo.Commit, vInfo.Date)
	}

	_, err := parser.ParseArgs(args[1:])
	if err != nil {
		if errFlag, ok := err.(*flags.Error); ok && askVersion && errFlag.Type == flags.ErrCommandRequired {
			return nil
		}
		if errFlag, ok := err.(*flags.Error); ok && errFlag.Type == flags.ErrCommandRequired {
			logrus.Error(err.Error())
			parser.WriteHelp(os.Stdout)
			os.Exit(0)
		}
		if errFlag, ok := err.(*flags.Error); ok && errFlag.Type == flags.ErrHelp {
			fmt.Println(err.Error())
			os.Exit(0)
		}
		return err
	}
	if mainOpts.Verbose {
		logrus.SetLevel(logrus.TraceLevel)
	}
	return nil
}

type varDefYaml struct {
	Name    string      `yaml:"name"`
	Type    string      `yaml:"type"`
	Options interface{} `yaml:"options"`
}

func (v varDefYaml) ToVariableDefinition() boshtpl.VariableDefinition {
	return boshtpl.VariableDefinition{
		Name:    v.Name,
		Type:    v.Type,
		Options: v.Options,
	}
}

func getVariableDefinitions() ([]boshtpl.VariableDefinition, error) {
	varDefsYaml := make([]varDefYaml, 0)
	b, err := ioutil.ReadFile(mainOpts.DefinitionFile)
	if err != nil {
		return []boshtpl.VariableDefinition{}, err
	}

	err = yaml.Unmarshal(b, &varDefsYaml)
	if err != nil {
		return []boshtpl.VariableDefinition{}, err
	}

	varDefs := make([]boshtpl.VariableDefinition, len(varDefsYaml))
	for i, v := range varDefsYaml {
		varDefs[i] = v.ToVariableDefinition()
	}
	return varDefs, nil
}

func run() error {
	err := Parse(VersionInfo{
		Version: version,
		Commit:  commit,
		Date:    date,
	}, os.Args)
	if err != nil {
		return err
	}

	varsDefs, err := getVariableDefinitions()
	if err != nil {
		return err
	}

	fsStore := NewVarsFSStore(mainOpts.StoreFile)
	fsStore.ValueGeneratorFactory = cfgtypes.NewValueGeneratorConcrete(NewVarsCertLoader(fsStore))

	return fsStore.LoadAndStore(varsDefs)
}

func main() {
	err := run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s", err.Error())
		os.Exit(1)
	}
}
