package charts

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/go-git/go-billy/v5"
	"github.com/rancher/charts-build-scripts/pkg/config"
	"github.com/rancher/charts-build-scripts/pkg/options"
	"github.com/rancher/charts-build-scripts/pkg/utils"
)

// GetPackages returns all packages found within the repository with the provided BranchOptions
// If there is a specific package provided, it will return just that Package in the list
func GetPackages(repoRoot string, specificPackage string, branchOpt options.BranchOptions) ([]*Package, error) {
	var packages []*Package
	repoFs := utils.GetFilesystem(repoRoot)
	if len(specificPackage) != 0 {
		pkg, err := GetPackage(repoFs, specificPackage, branchOpt)
		if err != nil {
			return nil, err
		}
		packages = append(packages, pkg)
		return packages, nil
	}
	fileInfos, err := repoFs.ReadDir(RepositoryPackagesDirpath)
	if err != nil {
		return nil, err
	}
	for _, fileInfo := range fileInfos {
		if !fileInfo.IsDir() {
			continue
		}
		name := fileInfo.Name()
		pkg, err := GetPackage(repoFs, name, branchOpt)
		if err != nil {
			return nil, err
		}
		packages = append(packages, pkg)
	}
	if len(packages) == 0 {
		return packages, fmt.Errorf("Could not find any packages in packages/")
	}
	return packages, nil
}

// GetPackage returns a Package based on the options provided
func GetPackage(repoFs billy.Filesystem, name string, branchOpt options.BranchOptions) (*Package, error) {
	// Get pkgFs
	packageRoot := filepath.Join(RepositoryPackagesDirpath, name)
	exists, err := utils.PathExists(repoFs, packageRoot)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, fmt.Errorf("Cannot find %s", packageRoot)
	}
	pkgFs, err := repoFs.Chroot(packageRoot)
	if err != nil {
		return nil, err
	}
	// Get package options from package.yaml
	packageOpt, err := options.LoadPackageOptionsFromFile(pkgFs, PackageOptionsFilepath)
	if err != nil {
		return nil, err
	}
	// Get charts
	chart, err := GetChartFromOptions(packageOpt.MainChartOptions)
	if err != nil {
		return nil, err
	}
	var additionalCharts []AdditionalChart
	for _, additionalChartOptions := range packageOpt.AdditionalChartOptions {
		additionalChart, err := GetAdditionalChartFromOptions(additionalChartOptions)
		if err != nil {
			return nil, err
		}
		additionalCharts = append(additionalCharts, additionalChart)
	}
	p := Package{
		Chart: chart,

		Name:             name,
		PackageVersion:   packageOpt.PackageVersion,
		BranchOptions:    branchOpt,
		AdditionalCharts: additionalCharts,

		fs:     pkgFs,
		repoFs: repoFs,
	}
	return &p, nil
}

// GetChartFromOptions returns a Chart based on the options provided
func GetChartFromOptions(opt options.ChartOptions) (Chart, error) {
	upstream, err := GetUpstream(opt.UpstreamOptions)
	if err != nil {
		return Chart{}, err
	}
	workingDir := opt.WorkingDir
	if len(workingDir) == 0 {
		workingDir = "charts"
	}
	return Chart{
		WorkingDir: workingDir,
		Upstream:   upstream,
	}, nil
}

// GetAdditionalChartFromOptions returns an AdditionalChart based on the options provided
func GetAdditionalChartFromOptions(opt options.AdditionalChartOptions) (AdditionalChart, error) {
	var a AdditionalChart
	if opt.UpstreamOptions != nil && opt.CRDChartOptions != nil {
		return a, fmt.Errorf("Invalid additional chart options provided: cannot define both UpstreamOptions and CRDChartOptions")
	}
	if opt.UpstreamOptions == nil && opt.CRDChartOptions == nil {
		return a, fmt.Errorf("Cannot parse additional chart options: you must either provide a URL (UpstreamOptions) or provide CRDChartOptions")
	}
	if len(opt.WorkingDir) == 0 {
		return a, fmt.Errorf("Cannot have additional chart without working directory")
	}
	if opt.WorkingDir == "charts" {
		return a, fmt.Errorf("Working directory for an additional chart cannot be charts")
	}
	a = AdditionalChart{
		WorkingDir: opt.WorkingDir,
	}
	if opt.UpstreamOptions != nil {
		upstream, err := GetUpstream(*opt.UpstreamOptions)
		if err != nil {
			return a, err
		}
		a.Upstream = &upstream
	}
	if opt.CRDChartOptions != nil {
		crdDirectory := opt.CRDChartOptions.CRDDirectory
		if len(crdDirectory) == 0 {
			return a, fmt.Errorf("CRD options must provide a directory to place CRDs within")
		}
		templateDirectory := opt.CRDChartOptions.TemplateDirectory
		if len(templateDirectory) == 0 {
			return a, fmt.Errorf("CRD options must provide a template directory")
		}
		a.CRDChartOptions = &options.CRDChartOptions{
			TemplateDirectory:           templateDirectory,
			CRDDirectory:                crdDirectory,
			AddCRDValidationToMainChart: opt.CRDChartOptions.AddCRDValidationToMainChart,
		}
	}
	return a, nil
}

// GetUpstream returns the appropriate Upstream given the options provided
func GetUpstream(opt options.UpstreamOptions) (Upstream, error) {
	if opt.URL == "" {
		return nil, fmt.Errorf("URL is not defined")
	}
	if strings.HasPrefix(opt.URL, "packages/") {
		upstream := UpstreamLocal{
			Name: strings.Split(opt.URL, "/")[1],
		}
		return upstream, nil
	}
	if strings.HasSuffix(opt.URL, ".git") {
		rc, err := config.GetRepositoryConfiguration(opt.URL)
		if err != nil {
			return nil, err
		}
		upstream := UpstreamRepository{RepositoryConfiguration: rc}
		if opt.Subdirectory != nil {
			upstream.Subdirectory = opt.Subdirectory
		}
		if opt.Commit != nil {
			upstream.Commit = opt.Commit
		}
		return upstream, nil
	}
	if strings.HasSuffix(opt.URL, ".tgz") || strings.Contains(opt.URL, ".tar.gz") {
		upstream := UpstreamChartArchive{
			URL: opt.URL,
		}
		if opt.Subdirectory != nil {
			upstream.Subdirectory = opt.Subdirectory
		}
		return upstream, nil
	}
	return nil, fmt.Errorf("URL is invalid (must contain .git or .tgz)")
}
