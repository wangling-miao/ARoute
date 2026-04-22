package sdk

import (
	"fmt"

	"github.com/wangling-miao/aroute/core"
	"github.com/wangling-miao/aroute/core/events"
	"github.com/wangling-miao/aroute/sdk/interfaces"
)

// GetDB retrieves the DatabaseService from the service container.
// Returns an error if the database plugin is not registered.
//
// Usage:
//
//	db, err := sdk.GetDB(ctx.Services())
//	if err != nil {
//	    return fmt.Errorf("database not available: %w", err)
//	}
//	rows, err := db.Query(context.Background(), "SELECT * FROM posts")
func GetDB(container core.ServiceContainer) (interfaces.DatabaseService, error) {
	var svc interfaces.DatabaseService
	if err := container.Get(&svc); err != nil {
		return nil, fmt.Errorf("sdk: database service not available: %w", err)
	}
	return svc, nil
}

// MustGetDB retrieves the DatabaseService, panicking if not available.
// Use only when the database is guaranteed to be present (programmer error if missing).
func MustGetDB(container core.ServiceContainer) interfaces.DatabaseService {
	svc, err := GetDB(container)
	if err != nil {
		panic(err)
	}
	return svc
}

// GetAuth retrieves the AuthService from the service container.
// Returns an error if the auth plugin is not registered.
//
// Usage:
//
//	auth, err := sdk.GetAuth(ctx.Services())
//	if err != nil {
//	    return fmt.Errorf("auth not available: %w", err)
//	}
//	claims, err := auth.VerifyToken(context.Background(), token)
func GetAuth(container core.ServiceContainer) (interfaces.AuthService, error) {
	var svc interfaces.AuthService
	if err := container.Get(&svc); err != nil {
		return nil, fmt.Errorf("sdk: auth service not available: %w", err)
	}
	return svc, nil
}

// MustGetAuth retrieves the AuthService, panicking if not available.
func MustGetAuth(container core.ServiceContainer) interfaces.AuthService {
	svc, err := GetAuth(container)
	if err != nil {
		panic(err)
	}
	return svc
}

// GetContent retrieves the ContentService from the service container.
// Returns an error if the content plugin is not registered.
//
// Usage:
//
//	content, err := sdk.GetContent(ctx.Services())
//	if err != nil {
//	    return fmt.Errorf("content service not available: %w", err)
//	}
//	post, err := content.Create(context.Background(), "post", data)
func GetContent(container core.ServiceContainer) (interfaces.ContentService, error) {
	var svc interfaces.ContentService
	if err := container.Get(&svc); err != nil {
		return nil, fmt.Errorf("sdk: content service not available: %w", err)
	}
	return svc, nil
}

// MustGetContent retrieves the ContentService, panicking if not available.
func MustGetContent(container core.ServiceContainer) interfaces.ContentService {
	svc, err := GetContent(container)
	if err != nil {
		panic(err)
	}
	return svc
}

// GetMedia retrieves the MediaService from the service container.
// Returns an error if the media plugin is not registered.
//
// Usage:
//
//	media, err := sdk.GetMedia(ctx.Services())
//	if err != nil {
//	    return fmt.Errorf("media service not available: %w", err)
//	}
//	file, err := media.GetByID(context.Background(), fileID)
func GetMedia(container core.ServiceContainer) (interfaces.MediaService, error) {
	var svc interfaces.MediaService
	if err := container.Get(&svc); err != nil {
		return nil, fmt.Errorf("sdk: media service not available: %w", err)
	}
	return svc, nil
}

// MustGetMedia retrieves the MediaService, panicking if not available.
func MustGetMedia(container core.ServiceContainer) interfaces.MediaService {
	svc, err := GetMedia(container)
	if err != nil {
		panic(err)
	}
	return svc
}

// GetSearch retrieves the SearchService from the service container.
// Returns an error if the search plugin is not registered.
//
// Usage:
//
//	search, err := sdk.GetSearch(ctx.Services())
//	if err != nil {
//	    return fmt.Errorf("search service not available: %w", err)
//	}
//	results, err := search.Search(context.Background(), query)
func GetSearch(container core.ServiceContainer) (interfaces.SearchService, error) {
	var svc interfaces.SearchService
	if err := container.Get(&svc); err != nil {
		return nil, fmt.Errorf("sdk: search service not available: %w", err)
	}
	return svc, nil
}

// MustGetSearch retrieves the SearchService, panicking if not available.
func MustGetSearch(container core.ServiceContainer) interfaces.SearchService {
	svc, err := GetSearch(container)
	if err != nil {
		panic(err)
	}
	return svc
}

// GetCache retrieves the CacheService from the service container.
// Returns an error if the cache plugin is not registered.
//
// Usage:
//
//	cache, err := sdk.GetCache(ctx.Services())
//	if err != nil {
//	    return fmt.Errorf("cache service not available: %w", err)
//	}
//	val, ok := cache.Get(context.Background(), "content:post:123")
func GetCache(container core.ServiceContainer) (interfaces.CacheService, error) {
	var svc interfaces.CacheService
	if err := container.Get(&svc); err != nil {
		return nil, fmt.Errorf("sdk: cache service not available: %w", err)
	}
	return svc, nil
}

// MustGetCache retrieves the CacheService, panicking if not available.
func MustGetCache(container core.ServiceContainer) interfaces.CacheService {
	svc, err := GetCache(container)
	if err != nil {
		panic(err)
	}
	return svc
}

// GetQueue retrieves the QueueService from the service container.
// Returns an error if the queue plugin is not registered.
//
// Usage:
//
//	queue, err := sdk.GetQueue(ctx.Services())
//	if err != nil {
//	    return fmt.Errorf("queue service not available: %w", err)
//	}
//	taskID, err := queue.Enqueue(context.Background(), "email.send", payload, nil)
func GetQueue(container core.ServiceContainer) (interfaces.QueueService, error) {
	var svc interfaces.QueueService
	if err := container.Get(&svc); err != nil {
		return nil, fmt.Errorf("sdk: queue service not available: %w", err)
	}
	return svc, nil
}

// MustGetQueue retrieves the QueueService, panicking if not available.
func MustGetQueue(container core.ServiceContainer) interfaces.QueueService {
	svc, err := GetQueue(container)
	if err != nil {
		panic(err)
	}
	return svc
}

// GetTheme retrieves the ThemeService from the service container.
// Returns an error if the theme plugin is not registered.
//
// Usage:
//
//	theme, err := sdk.GetTheme(ctx.Services())
//	if err != nil {
//	    return fmt.Errorf("theme service not available: %w", err)
//	}
//	html, err := theme.Render(context.Background(), "post.html", data)
func GetTheme(container core.ServiceContainer) (interfaces.ThemeService, error) {
	var svc interfaces.ThemeService
	if err := container.Get(&svc); err != nil {
		return nil, fmt.Errorf("sdk: theme service not available: %w", err)
	}
	return svc, nil
}

// MustGetTheme retrieves the ThemeService, panicking if not available.
func MustGetTheme(container core.ServiceContainer) interfaces.ThemeService {
	svc, err := GetTheme(container)
	if err != nil {
		panic(err)
	}
	return svc
}

// GetRouter retrieves the RouteRegistrar from the service container.
// Returns an error if the HTTP plugin is not registered.
// Use this to register custom HTTP routes from your plugin.
//
// Usage:
//
//	router, err := sdk.GetRouter(ctx.Services())
//	if err != nil {
//	    return fmt.Errorf("router not available: %w", err)
//	}
//	router.Route("/api/plugins/my-plugin", func(r chi.Router) {
//	    r.Get("/hello", myHandler)
//	})
func GetRouter(container core.ServiceContainer) (interfaces.RouteRegistrar, error) {
	var svc interfaces.RouteRegistrar
	if err := container.Get(&svc); err != nil {
		return nil, fmt.Errorf("sdk: route registrar not available: %w", err)
	}
	return svc, nil
}

// MustGetRouter retrieves the RouteRegistrar, panicking if not available.
func MustGetRouter(container core.ServiceContainer) interfaces.RouteRegistrar {
	svc, err := GetRouter(container)
	if err != nil {
		panic(err)
	}
	return svc
}

// SubscribeEvent is a convenience helper that subscribes to a broadcast event.
// It wraps the EventBus.SubscribeBroadcast call with a typed handler function.
// Returns a handler ID for later unsubscription.
//
// Usage:
//
//	handlerID := sdk.SubscribeEvent(ctx, "content.post.created", func(ctx context.Context, event events.Event) {
//	    log.Printf("Post created: %v", event.Data["id"])
//	})
//	defer ctx.Events().Unsubscribe(handlerID)
func SubscribeEvent(ctx core.CoreContext, topic string, handler events.BroadcastHandler) string {
	return ctx.Events().SubscribeBroadcast(topic, handler)
}

// OnContentCreated subscribes to content creation events for all or specific content types.
// The handler receives the raw events.Event, from which you can read Data["id"], Data["content_type"], etc.
// Returns a handler ID for later unsubscription.
//
// Usage:
//
//	handlerID := sdk.OnContentCreated(ctx, "post", func(ctx context.Context, event events.Event) {
//	    log.Printf("New post created: %s", event.Data["id"])
//	})
//	defer ctx.Events().Unsubscribe(handlerID)
func OnContentCreated(ctx core.CoreContext, contentType string, handler events.BroadcastHandler) string {
	topic := "content.*.created"
	if contentType != "" {
		topic = "content." + contentType + ".created"
	}

	return ctx.Events().SubscribeBroadcast(topic, handler)
}
