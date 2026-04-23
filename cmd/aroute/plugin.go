// Package main implements the plugin subcommand for ARoute CMS.
package main

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"github.com/wangling-miao/aroute/core"
	"github.com/wangling-miao/aroute/core/registry"
)

var pluginCmd = &cobra.Command{
	Use:   "plugin",
	Short: "Manage plugins",
	Long: `List, install, enable, disable, and remove plugins.

Plugins are stored in the plugins directory and their metadata is
tracked in the plugin registry (data/registry.db).`,
}

var pluginListCmd = &cobra.Command{
	Use:   "list",
	Short: "List installed plugins",
	Long:  `Display a table of all installed plugins with their status.`,
	RunE:  runPluginList,
}

var pluginInstallCmd = &cobra.Command{
	Use:   "install <path>",
	Short: "Install a plugin from a local path",
	Long: `Install a plugin from a local directory or tarball.

The plugin must contain a valid manifest.yaml or manifest.json file.`,
	Args: cobra.ExactArgs(1),
	RunE: runPluginInstall,
}

var pluginEnableCmd = &cobra.Command{
	Use:   "enable <name>",
	Short: "Enable a plugin",
	Long:  `Mark a plugin as enabled in the registry.`,
	Args:  cobra.ExactArgs(1),
	RunE:  runPluginEnable,
}

var pluginDisableCmd = &cobra.Command{
	Use:   "disable <name>",
	Short: "Disable a plugin",
	Long:  `Mark a plugin as disabled in the registry.`,
	Args:  cobra.ExactArgs(1),
	RunE:  runPluginDisable,
}

var pluginRemoveCmd = &cobra.Command{
	Use:   "remove <name>",
	Short: "Remove a plugin",
	Long:  `Remove a plugin from the registry and delete its files.`,
	Args:  cobra.ExactArgs(1),
	RunE:  runPluginRemove,
}

func init() {
	rootCmd.AddCommand(pluginCmd)
	pluginCmd.AddCommand(pluginListCmd)
	pluginCmd.AddCommand(pluginInstallCmd)
	pluginCmd.AddCommand(pluginEnableCmd)
	pluginCmd.AddCommand(pluginDisableCmd)
	pluginCmd.AddCommand(pluginRemoveCmd)
}

// openRegistry creates a BoltUnifiedRegistry wrapped as a legacy Registry.
func openRegistry(registryPath string) (*registry.LegacyRegistry, error) {
	unifiedReg, err := registry.NewBoltUnifiedRegistry(registryPath)
	if err != nil {
		return nil, err
	}
	return registry.NewLegacyRegistry(unifiedReg), nil
}

func runPluginList(cmd *cobra.Command, args []string) error {
	dataDir := getDataDir()
	registryPath := filepath.Join(dataDir, "registry.db")

	reg, err := openRegistry(registryPath)
	if err != nil {
		return listFromManifests()
	}
	defer reg.Close()

	entries, err := reg.List()
	if err != nil {
		return fmt.Errorf("list plugins: %w", err)
	}

	if len(entries) == 0 {
		fmt.Println("No plugins installed.")
		return nil
	}

	printPluginTable(entries)
	return nil
}

func listFromManifests() error {
	pluginDir := getPluginDir()
	discovery := registry.NewFSDiscovery(pluginDir)
	paths, err := discovery.Discover()
	if err != nil {
		return fmt.Errorf("discover plugins: %w", err)
	}

	if len(paths) == 0 {
		fmt.Println("No plugins installed.")
		return nil
	}

	var entries []*registry.PluginEntry
	for name, manifestPath := range paths {
		manifest, err := core.LoadManifest(manifestPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: invalid manifest for %s: %v\n", name, err)
			continue
		}
		entries = append(entries, &registry.PluginEntry{
			Manifest:       *manifest,
			Enabled:        true,
			DiscoveredPath: manifestPath,
		})
	}

	fmt.Fprintln(os.Stderr, "(registry locked — showing manifests from disk, status always 'enabled')")
	printPluginTable(entries)
	return nil
}

func printPluginTable(entries []*registry.PluginEntry) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tVERSION\tSTATUS\tAUTHOR")
	fmt.Fprintln(w, "----\t-------\t------\t------")

	for _, entry := range entries {
		status := "disabled"
		if entry.Enabled {
			status = "enabled"
		}
		author := entry.Manifest.Author
		if author == "" {
			author = "unknown"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
			entry.Manifest.Name,
			entry.Manifest.Version,
			status,
			author,
		)
	}
	w.Flush()
}

const maxPluginSize = 100 * 1024 * 1024 // 100MB

func installPluginFromURL(rawURL, pluginDir, registryPath string) error {
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return fmt.Errorf("unsupported URL scheme %q: only http and https are allowed", parsedURL.Scheme)
	}

	fmt.Printf("Downloading plugin from %s...\n", rawURL)

	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Get(rawURL)
	if err != nil {
		return fmt.Errorf("download plugin: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed: HTTP %d", resp.StatusCode)
	}

	tmpDir, err := os.MkdirTemp("", "aroute-plugin-*")
	if err != nil {
		return fmt.Errorf("create temp directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	if strings.HasSuffix(rawURL, ".tar.gz") || strings.HasSuffix(rawURL, ".tgz") {
		tmpFile := filepath.Join(tmpDir, "plugin.tar.gz")
		data, err := io.ReadAll(io.LimitReader(resp.Body, maxPluginSize+1))
		if err != nil {
			return fmt.Errorf("read download: %w", err)
		}
		if len(data) > maxPluginSize {
			return fmt.Errorf("plugin archive exceeds maximum size of %d bytes", maxPluginSize)
		}
		if err := os.WriteFile(tmpFile, data, 0644); err != nil {
			return fmt.Errorf("save download: %w", err)
		}
		if err := extractTarGz(tmpFile, tmpDir); err != nil {
			return fmt.Errorf("extract archive: %w", err)
		}
	} else {
		return fmt.Errorf("unsupported archive format: only .tar.gz and .tgz are supported")
	}

	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		return fmt.Errorf("read extracted directory: %w", err)
	}

	var pluginSourcePath string
	for _, entry := range entries {
		if entry.IsDir() {
			pluginSourcePath = filepath.Join(tmpDir, entry.Name())
			break
		}
	}

	if pluginSourcePath == "" {
		return fmt.Errorf("no plugin directory found in archive")
	}

	return installPluginFromPath(pluginSourcePath, pluginDir, registryPath)
}

func extractTarGz(tarGzPath, destDir string) error {
	f, err := os.Open(tarGzPath)
	if err != nil {
		return fmt.Errorf("open archive: %w", err)
	}
	defer f.Close()

	gzr, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("decompress archive: %w", err)
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read tar entry: %w", err)
		}

		cleanedName := filepath.Clean(hdr.Name)
		if strings.Contains(cleanedName, "..") || filepath.IsAbs(cleanedName) {
			fmt.Fprintf(os.Stderr, "Warning: skipping unsafe path in archive: %s\n", hdr.Name)
			continue
		}
		fullPath := filepath.Join(destDir, cleanedName)
		if !strings.HasPrefix(fullPath, destDir+string(os.PathSeparator)) && fullPath != destDir {
			fmt.Fprintf(os.Stderr, "Warning: skipping path escaping dest dir: %s\n", hdr.Name)
			continue
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(fullPath, os.FileMode(hdr.Mode)); err != nil {
				return fmt.Errorf("create directory %s: %w", cleanedName, err)
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
				return fmt.Errorf("create parent directory for %s: %w", cleanedName, err)
			}
			outFile, err := os.OpenFile(fullPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(hdr.Mode))
			if err != nil {
				return fmt.Errorf("create file %s: %w", cleanedName, err)
			}
			if _, err := io.Copy(outFile, io.LimitReader(tr, maxPluginSize)); err != nil {
				outFile.Close()
				return fmt.Errorf("write file %s: %w", cleanedName, err)
			}
			outFile.Close()
		default:
			fmt.Fprintf(os.Stderr, "Warning: skipping unsupported tar entry type %d: %s\n", hdr.Typeflag, hdr.Name)
		}
	}
	return nil
}

func runPluginInstall(cmd *cobra.Command, args []string) error {
	sourcePath := args[0]
	dataDir := getDataDir()
	pluginDir := getPluginDir()
	registryPath := filepath.Join(dataDir, "registry.db")

	if strings.HasPrefix(sourcePath, "http://") || strings.HasPrefix(sourcePath, "https://") {
		return installPluginFromURL(sourcePath, pluginDir, registryPath)
	}

	return installPluginFromPath(sourcePath, pluginDir, registryPath)
}

func installPluginFromPath(sourcePath, pluginDir, registryPath string) error {

	// Check if source is a valid plugin directory
	manifestPath := filepath.Join(sourcePath, "manifest.yaml")
	if _, err := os.Stat(manifestPath); os.IsNotExist(err) {
		// Try manifest.json
		manifestPath = filepath.Join(sourcePath, "manifest.json")
		if _, err := os.Stat(manifestPath); os.IsNotExist(err) {
			return fmt.Errorf("manifest.yaml or manifest.json not found in %s", sourcePath)
		}
	}

	// Load manifest
	manifest, err := core.LoadManifest(manifestPath)
	if err != nil {
		return fmt.Errorf("load manifest: %w", err)
	}

	// Open registry
	reg, err := openRegistry(registryPath)
	if err != nil {
		return fmt.Errorf("open registry: %w", err)
	}
	defer reg.Close()

	// Check if already installed
	if _, err := reg.Get(manifest.Name); err == nil {
		return fmt.Errorf("plugin '%s' is already installed. Use 'aroute plugin remove %s' first.", manifest.Name, manifest.Name)
	}

	// Copy plugin to plugins directory
	targetPath := filepath.Join(pluginDir, manifest.Name)
	if err := copyPlugin(sourcePath, targetPath); err != nil {
		return fmt.Errorf("copy plugin: %w", err)
	}

	// Register in registry
	entry := &registry.PluginEntry{
		Manifest:       *manifest,
		Enabled:        false, // Plugins start disabled
		DiscoveredPath: targetPath,
	}
	if err := reg.Register(entry); err != nil {
		return fmt.Errorf("register plugin: %w", err)
	}

	fmt.Printf("Installed plugin: %s v%s\n", manifest.Name, manifest.Version)
	fmt.Println("To enable it, run: aroute plugin enable " + manifest.Name)

	return nil
}

func runPluginEnable(cmd *cobra.Command, args []string) error {
	pluginName := args[0]
	dataDir := getDataDir()
	registryPath := filepath.Join(dataDir, "registry.db")

	reg, err := openRegistry(registryPath)
	if err != nil {
		return fmt.Errorf("open registry: %w", err)
	}
	defer reg.Close()

	// Check if plugin exists
	_, err = reg.Get(pluginName)
	if err != nil {
		return fmt.Errorf("plugin '%s' is not installed", pluginName)
	}

	if err := reg.Enable(pluginName); err != nil {
		return fmt.Errorf("enable plugin: %w", err)
	}

	fmt.Printf("Enabled plugin: %s\n", pluginName)
	return nil
}

func runPluginDisable(cmd *cobra.Command, args []string) error {
	pluginName := args[0]
	dataDir := getDataDir()
	registryPath := filepath.Join(dataDir, "registry.db")

	reg, err := openRegistry(registryPath)
	if err != nil {
		return fmt.Errorf("open registry: %w", err)
	}
	defer reg.Close()

	// Check if plugin exists
	_, err = reg.Get(pluginName)
	if err != nil {
		return fmt.Errorf("plugin '%s' is not installed", pluginName)
	}

	if err := reg.Disable(pluginName); err != nil {
		return fmt.Errorf("disable plugin: %w", err)
	}

	fmt.Printf("Disabled plugin: %s\n", pluginName)
	return nil
}

func runPluginRemove(cmd *cobra.Command, args []string) error {
	pluginName := args[0]
	dataDir := getDataDir()
	pluginDir := getPluginDir()
	registryPath := filepath.Join(dataDir, "registry.db")

	reg, err := openRegistry(registryPath)
	if err != nil {
		return fmt.Errorf("open registry: %w", err)
	}
	defer reg.Close()

	// Get plugin entry to find its path
	entry, err := reg.Get(pluginName)
	if err != nil {
		return fmt.Errorf("plugin '%s' is not installed", pluginName)
	}

	// Remove from registry
	if err := reg.Remove(pluginName); err != nil {
		return fmt.Errorf("remove from registry: %w", err)
	}

	// Remove plugin files
	pluginPath := filepath.Join(pluginDir, pluginName)
	if entry.DiscoveredPath != "" {
		pluginPath = entry.DiscoveredPath
	}
	if err := os.RemoveAll(pluginPath); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not remove plugin files: %v\n", err)
	}

	fmt.Printf("Removed plugin: %s\n", pluginName)
	return nil
}

// copyPlugin copies a plugin directory to the plugins directory
func copyPlugin(src, dst string) error {
	// Ensure destination parent exists
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}

	// Simple recursive copy
	return copyDir(src, dst)
}

// copyDir recursively copies a directory
func copyDir(src, dst string) error {
	si, err := os.Stat(src)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(dst, si.Mode()); err != nil {
		return err
	}

	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		info, err := os.Lstat(srcPath)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			fmt.Fprintf(os.Stderr, "Warning: skipping symlink: %s\n", srcPath)
			continue
		}

		if info.IsDir() {
			if err := copyDir(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			data, err := os.ReadFile(srcPath)
			if err != nil {
				return err
			}
			if err := os.WriteFile(dstPath, data, 0644); err != nil {
				return err
			}
		}
	}

	return nil
}
