package cnabprovider

import (
	"encoding/base64"
	"encoding/json"
	"fmt"

	"get.porter.sh/porter/pkg/parameters"
	"github.com/cnabio/cnab-go/bundle"
	"github.com/cnabio/cnab-go/bundle/definition"
	"github.com/cnabio/cnab-go/valuesource"
	"github.com/pkg/errors"
)

// loadParameters accepts a set of parameter overrides as well as parameter set
// files and combines both with the default parameters to create a full set
// of parameters.
func (r *Runtime) loadParameters(bun bundle.Bundle, rawOverrides map[string]string, parameterSets []string, action string) (map[string]interface{}, error) {
	overrides := make(map[string]interface{}, len(rawOverrides))

	// Loop through each parameter set file and load the parameter values
	loaded, err := r.loadParameterSets(parameterSets)
	if err != nil {
		return nil, errors.Wrapf(err, "unable to process provided parameter sets: %v", parameterSets)
	}

	for key, val := range loaded {
		overrides[key] = val
	}

	// Now give precedence to the raw overrides that came via the CLI
	for key, rawValue := range rawOverrides {
		param, ok := bun.Parameters[key]
		if !ok {
			return nil, fmt.Errorf("parameter %s not defined in bundle", key)
		}

		def, ok := bun.Definitions[param.Definition]
		if !ok {
			return nil, fmt.Errorf("definition %s not defined in bundle", param.Definition)
		}

		unconverted, err := r.getUnconvertedValueFromRaw(def, key, rawValue)
		if err != nil {
			return nil, err
		}

		value, err := def.ConvertValue(unconverted)
		if err != nil {
			return nil, errors.Wrapf(err, "unable to convert parameter's %s value %s to the destination parameter type %s", key, rawValue, def.Type)
		}

		overrides[key] = value
	}

	return bundle.ValuesOrDefaults(overrides, &bun, action)
}

// loadParameterSets loads parameter values per their parameter set strategies
func (r *Runtime) loadParameterSets(params []string) (valuesource.Set, error) {
	resolvedParameters := valuesource.Set{}
	for _, name := range params {
		var pset parameters.ParameterSet
		var err error
		if r.isPathy(name) {
			pset, err = r.loadParameterFromFile(name)
		} else {
			pset, err = r.parameters.Read(name)
		}
		if err != nil {
			return nil, err
		}

		rc, err := r.parameters.ResolveAll(pset)
		if err != nil {
			return nil, err
		}

		for k, v := range rc {
			resolvedParameters[k] = v
		}
	}

	return resolvedParameters, nil
}

func (r *Runtime) loadParameterFromFile(path string) (parameters.ParameterSet, error) {
	data, err := r.FileSystem.ReadFile(path)
	if err != nil {
		return parameters.ParameterSet{}, errors.Wrapf(err, "could not read file %s", path)
	}

	var cs parameters.ParameterSet
	err = json.Unmarshal(data, &cs)
	return cs, errors.Wrapf(err, "error loading parameter set in %s", path)
}

func (r *Runtime) getUnconvertedValueFromRaw(def *definition.Schema, key, rawValue string) (string, error) {
	// the parameter value (via rawValue) may represent a file on the local filesystem
	if def.Type == "string" && def.ContentEncoding == "base64" {
		if _, err := r.FileSystem.Stat(rawValue); err == nil {
			bytes, err := r.FileSystem.ReadFile(rawValue)
			if err != nil {
				return "", errors.Wrapf(err, "unable to read file parameter %s", key)
			}
			return base64.StdEncoding.EncodeToString(bytes), nil
		}
	}
	return rawValue, nil
}
