package apix

import (
	"sort"
	"sync"

	"github.com/Infra-Forge/infra-apix/internal/generation"
	"github.com/Infra-Forge/infra-apix/internal/logging"
	"github.com/getkin/kin-openapi/openapi3"
)

// Plugin defines hooks for customizing route metadata and OpenAPI spec generation.
// Plugins can be registered globally to inject custom metadata, transform schemas,
// or modify the final OpenAPI document.
type Plugin interface {
	// Name returns the unique identifier for this plugin.
	Name() string

	// OnRouteRegister is called when a route is registered, before it's added to the registry.
	// Plugins can modify the RouteRef to add custom metadata, tags, or parameters.
	// Return an error to prevent route registration.
	OnRouteRegister(ref *RouteRef) error

	// OnSchemaGenerate is called once per build for each named struct type,
	// after its component schema is generated. Inline structs, primitives,
	// slices and maps do not reach it; OnSpecBuild sees the whole document.
	// Plugins can modify the schema to add custom properties, validation, or extensions.
	// The typeName parameter is the component name (e.g., "models_User" for models.User).
	OnSchemaGenerate(typeName string, schema *openapi3.Schema) error

	// OnSpecBuild is called after the OpenAPI document is built, before it's returned.
	// Plugins can modify the document to add custom extensions, security schemes, or servers.
	OnSpecBuild(doc *openapi3.T) error
}

// pluginRegistry holds all registered plugins.
var pluginRegistry = &PluginRegistry{
	plugins: make(map[string]Plugin),
}

// PluginRegistry manages registered plugins.
type PluginRegistry struct {
	mu      sync.RWMutex
	plugins map[string]Plugin
}

// RegisterPlugin adds a plugin to the global registry.
// If a plugin with the same name already exists, it will be replaced.
func RegisterPlugin(p Plugin) {
	if p == nil {
		return
	}
	pluginRegistry.mu.Lock()
	defer pluginRegistry.mu.Unlock()
	pluginRegistry.plugins[p.Name()] = p
	generation.Advance()
	logging.GetLogger().PluginRegistered(p.Name())
}

// UnregisterPlugin removes a plugin from the global registry.
func UnregisterPlugin(name string) {
	pluginRegistry.mu.Lock()
	defer pluginRegistry.mu.Unlock()
	delete(pluginRegistry.plugins, name)
	generation.Advance()
}

// GetPlugin retrieves a plugin by name.
func GetPlugin(name string) (Plugin, bool) {
	pluginRegistry.mu.RLock()
	defer pluginRegistry.mu.RUnlock()
	p, ok := pluginRegistry.plugins[name]
	return p, ok
}

// ListPlugins returns all registered plugin names.
func ListPlugins() []string {
	pluginRegistry.mu.RLock()
	defer pluginRegistry.mu.RUnlock()
	names := make([]string, 0, len(pluginRegistry.plugins))
	for name := range pluginRegistry.plugins {
		names = append(names, name)
	}
	return names
}

// ResetPlugins clears all registered plugins.
// This is primarily useful for testing.
func ResetPlugins() {
	pluginRegistry.mu.Lock()
	defer pluginRegistry.mu.Unlock()
	pluginRegistry.plugins = make(map[string]Plugin)
	generation.Advance()
}

// getPluginsSorted returns a sorted slice of plugins for deterministic execution.
// The caller must hold at least a read lock on pluginRegistry.mu.
func getPluginsSorted() []Plugin {
	plugins := make([]Plugin, 0, len(pluginRegistry.plugins))
	for _, p := range pluginRegistry.plugins {
		plugins = append(plugins, p)
	}
	// Sort by plugin name for deterministic order
	sort.Slice(plugins, func(i, j int) bool {
		return plugins[i].Name() < plugins[j].Name()
	})
	return plugins
}

// executeOnRouteRegister calls OnRouteRegister for all registered plugins in deterministic order.
func executeOnRouteRegister(ref *RouteRef) error {
	pluginRegistry.mu.RLock()
	plugins := getPluginsSorted()
	pluginRegistry.mu.RUnlock()

	for _, plugin := range plugins {
		logging.GetLogger().PluginExecuted(plugin.Name(), "OnRouteRegister")
		if err := plugin.OnRouteRegister(ref); err != nil {
			return err
		}
	}
	return nil
}

// ExecuteOnSchemaGenerate calls OnSchemaGenerate for all registered plugins in deterministic order.
// This is exported for use by the OpenAPI builder.
func ExecuteOnSchemaGenerate(typeName string, schema *openapi3.Schema) error {
	pluginRegistry.mu.RLock()
	plugins := getPluginsSorted()
	pluginRegistry.mu.RUnlock()

	for _, plugin := range plugins {
		logging.GetLogger().PluginExecuted(plugin.Name(), "OnSchemaGenerate", "type", typeName)
		if err := plugin.OnSchemaGenerate(typeName, schema); err != nil {
			return err
		}
	}
	return nil
}

// ExecuteOnSpecBuild calls OnSpecBuild for all registered plugins in deterministic order.
// This is exported for use by the OpenAPI builder.
func ExecuteOnSpecBuild(doc *openapi3.T) error {
	pluginRegistry.mu.RLock()
	plugins := getPluginsSorted()
	pluginRegistry.mu.RUnlock()

	for _, plugin := range plugins {
		logging.GetLogger().PluginExecuted(plugin.Name(), "OnSpecBuild")
		if err := plugin.OnSpecBuild(doc); err != nil {
			return err
		}
	}
	return nil
}

// BasePlugin provides a default implementation of the Plugin interface.
// Embed this in your custom plugins to only implement the hooks you need.
type BasePlugin struct {
	PluginName string
}

// Name returns the plugin name.
func (p *BasePlugin) Name() string {
	return p.PluginName
}

// OnRouteRegister is a no-op by default.
func (p *BasePlugin) OnRouteRegister(ref *RouteRef) error {
	return nil
}

// OnSchemaGenerate is a no-op by default.
func (p *BasePlugin) OnSchemaGenerate(typeName string, schema *openapi3.Schema) error {
	return nil
}

// OnSpecBuild is a no-op by default.
func (p *BasePlugin) OnSpecBuild(doc *openapi3.T) error {
	return nil
}
