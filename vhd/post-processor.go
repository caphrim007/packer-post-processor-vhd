// Package vhd implements the packer.PostProcessor interface and adds a
// post-processor that produces a standalone VHD file.
package vhd

import (
	"fmt"
	"os"

	"github.com/mitchellh/packer/builder/qemu"
	vboxcommon "github.com/mitchellh/packer/builder/virtualbox/common"
	"github.com/mitchellh/packer/common"
	"github.com/mitchellh/packer/helper/config"
	"github.com/mitchellh/packer/packer"
	"github.com/mitchellh/packer/template/interpolate"
)

// Map Builders to Providers: these are the types of artifacts we know how to
// convert.
var providers = map[string]Provider{
	vboxcommon.BuilderId: NewVirtualBoxProvider(),
	qemu.BuilderId:       NewQEMUProvider(),
}

// Config contains the post-processor configuration.
type Config struct {
	common.PackerConfig `mapstructure:",squash"`

	// Where the VHD will be output to.
	OutputPath string `mapstructure:"output"`

	// Whether to keep the Provider artifact (e.g., VirtualBox VMDK).
	KeepInputArtifact bool `mapstructure:"keep_input_artifict"`

	// Whether to overwrite the VHD if it exists.
	Force bool `mapstructure:"force"`

	ctx interpolate.Context
}

// PostProcessor satisfies the packer.PostProcessor interface.
type PostProcessor struct {
	config Config
}

// outputPathTemplate contains fields available to template the VHD output
// path.
type outputPathTemplate struct {
	ArtifactId string
	BuildName  string
	Provider   string
}

// Configure the PostProcessor, rendering templated values if necessary.
func (p *PostProcessor) Configure(raws ...interface{}) error {
	err := config.Decode(&p.config, &config.DecodeOpts{
		Interpolate: true,
		InterpolateFilter: &interpolate.RenderFilter{
			Exclude: []string{},
		},
	}, raws...)
	if err != nil {
		return err
	}

	if p.config.OutputPath == "" {
		p.config.OutputPath = "packer_{{ .BuildName }}_{{ .Provider }}.vhd"
	}

	return nil
}

// PostProcess is the main entry point. It calls a Provider's Convert() method
// to delegate conversion to that Provider's command-line tool.
func (p *PostProcessor) PostProcess(ui packer.Ui, artifact packer.Artifact) (packer.Artifact, bool, error) {
	provider, err := providerForBuilderId(artifact.BuilderId())
	if err != nil {
		return nil, false, err
	}

	ui.Say(fmt.Sprintf("Converting %s image to VHD file...", provider))

	// Render output path.
	p.config.ctx.Data = &outputPathTemplate{
		ArtifactId: artifact.Id(),
		BuildName:  p.config.PackerBuildName,
		Provider:   provider.Name(),
	}
	outputPath, err := interpolate.Render(p.config.OutputPath, &p.config.ctx)
	if err != nil {
		return nil, false, err
	}

	// Check if VHD file exists. Remove if the user specified `force` in the
	// template or `--force` on the command-line.
	// This differs from the Vagrant post-processor because the the VHD can be
	// used (and written to) immediately. It is comparable to a Builder
	// end-product.
	if _, err = os.Stat(outputPath); err == nil {
		if p.config.PackerForce || p.config.Force {
			ui.Message(fmt.Sprintf("Removing existing VHD file at %s", outputPath))
			os.Remove(outputPath)
		} else {
			return nil, false, fmt.Errorf("VHD file exists: %s\nUse the force flag to delete it.", outputPath)
		}
	}

	err = provider.Convert(ui, artifact, outputPath)
	if err != nil {
		return nil, false, err
	}

	ui.Say(fmt.Sprintf("Converted VHD: %s", outputPath))
	artifact = NewArtifact(provider.String(), outputPath)
	keep := p.config.KeepInputArtifact

	return artifact, keep, nil
}

// Pick a provider to use from known builder sources.
func providerForBuilderId(builderId string) (Provider, error) {
	if provider, ok := providers[builderId]; ok {
		return provider, nil
	}
	return nil, fmt.Errorf("Unknown artifact type: %s", builderId)
}
