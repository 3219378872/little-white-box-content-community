package tool

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"errx"
	"esx/app/content/rpc/contentservice"
	"esx/app/recommend/rpc/recommendservice"
	"esx/app/search/rpc/searchservice"
	"user/userservice"
)

type Name string

const (
	Search    Name = "search"
	Content   Name = "content"
	Recommend Name = "recommend"
	User      Name = "user"

	defaultMaxSources = 5
)

type Request struct {
	UserID    int64
	Message   string
	PostID    int64
	RequestID string
}

type Source struct {
	Type  string
	ID    string
	Title string
}

type Result struct {
	Text    string
	Sources []Source
}

type Executor interface {
	Execute(ctx context.Context, name Name, request Request) (*Result, error)
}

type Clients struct {
	Search    searchservice.SearchService
	Content   contentservice.ContentService
	Recommend recommendservice.RecommendService
	User      userservice.UserService
}

type handler func(context.Context, Request) (*Result, error)

type Registry struct {
	allowed  map[Name]struct{}
	handlers map[Name]handler
}

func NewRegistry(allowed []string, clients Clients, maxSources int) (*Registry, error) {
	if maxSources <= 0 {
		maxSources = defaultMaxSources
	}

	registry := &Registry{
		allowed: make(map[Name]struct{}, len(allowed)),
		handlers: map[Name]handler{
			Search:    searchHandler(clients.Search, maxSources),
			Content:   contentHandler(clients.Content),
			Recommend: recommendHandler(clients.Recommend, maxSources),
			User:      userHandler(clients.User),
		},
	}
	for _, configured := range allowed {
		name := Name(strings.TrimSpace(configured))
		if _, known := registry.handlers[name]; !known {
			return nil, fmt.Errorf("assistant: unknown allowed tool %q", configured)
		}
		registry.allowed[name] = struct{}{}
	}
	if len(registry.allowed) == 0 {
		return nil, fmt.Errorf("assistant: AllowedTools must contain at least one tool")
	}
	return registry, nil
}

func (r *Registry) Execute(ctx context.Context, name Name, request Request) (*Result, error) {
	if r == nil {
		return nil, errx.NewWithCode(errx.ServiceUnavailable)
	}
	if _, ok := r.allowed[name]; !ok {
		return nil, errx.New(errx.PermissionDenied, "assistant tool is not allowed")
	}
	handle, ok := r.handlers[name]
	if !ok {
		return nil, errx.NewWithCode(errx.ServiceUnavailable)
	}
	result, err := handle(ctx, request)
	if err != nil {
		return nil, err
	}
	if result == nil || strings.TrimSpace(result.Text) == "" {
		return nil, errx.New(errx.SystemError, "assistant tool returned no result")
	}
	return result, nil
}

func searchHandler(client searchservice.SearchService, maxSources int) handler {
	return func(ctx context.Context, request Request) (*Result, error) {
		if client == nil {
			return nil, errx.NewWithCode(errx.ServiceUnavailable)
		}
		response, err := client.Search(ctx, &searchservice.SearchReq{
			Keyword:  request.Message,
			Page:     1,
			PageSize: int32(maxSources),
		})
		if err != nil {
			return nil, err
		}
		if response == nil {
			return nil, errx.NewWithCode(errx.ServiceUnavailable)
		}

		sources := make([]Source, 0, maxSources)
		for _, post := range response.Posts {
			if post == nil || len(sources) >= maxSources {
				continue
			}
			sources = append(sources, Source{Type: "post", ID: strconv.FormatInt(post.Id, 10), Title: truncate(post.Title, 120)})
		}
		for _, user := range response.Users {
			if user == nil || len(sources) >= maxSources {
				continue
			}
			title := user.Nickname
			if title == "" {
				title = user.Username
			}
			sources = append(sources, Source{Type: "user", ID: strconv.FormatInt(user.Id, 10), Title: truncate(title, 120)})
		}
		for _, tag := range response.Tags {
			if tag == nil || len(sources) >= maxSources {
				continue
			}
			sources = append(sources, Source{Type: "tag", ID: tag.Name, Title: truncate(tag.Name, 120)})
		}

		total := len(response.Posts) + len(response.Users) + len(response.Tags)
		if total == 0 {
			return &Result{Text: fmt.Sprintf("No matching community content was found for %q.", truncate(request.Message, 80))}, nil
		}
		return &Result{
			Text:    fmt.Sprintf("Found %d posts, %d users, and %d tags for %q.", len(response.Posts), len(response.Users), len(response.Tags), truncate(request.Message, 80)),
			Sources: sources,
		}, nil
	}
}

func contentHandler(client contentservice.ContentService) handler {
	return func(ctx context.Context, request Request) (*Result, error) {
		if client == nil {
			return nil, errx.NewWithCode(errx.ServiceUnavailable)
		}
		if request.PostID <= 0 {
			return nil, errx.New(errx.ParamError, "a valid post id is required")
		}
		response, err := client.GetPost(ctx, &contentservice.GetPostReq{PostId: request.PostID, UserId: request.UserID})
		if err != nil {
			return nil, err
		}
		if response == nil || response.Post == nil {
			return nil, errx.NewWithCode(errx.ContentNotFound)
		}
		post := response.Post
		title := truncate(post.Title, 120)
		text := fmt.Sprintf("Post %d: %s", post.Id, title)
		if summary := truncate(strings.TrimSpace(post.Content), 240); summary != "" {
			text += "\n" + summary
		}
		return &Result{
			Text: text,
			Sources: []Source{{
				Type:  "post",
				ID:    strconv.FormatInt(post.Id, 10),
				Title: title,
			}},
		}, nil
	}
}

func recommendHandler(client recommendservice.RecommendService, maxSources int) handler {
	return func(ctx context.Context, request Request) (*Result, error) {
		if client == nil {
			return nil, errx.NewWithCode(errx.ServiceUnavailable)
		}
		response, err := client.GetRecommendPosts(ctx, &recommendservice.GetRecommendPostsReq{
			UserId:    request.UserID,
			Scene:     "assistant",
			RequestId: request.RequestID,
			PageSize:  int32(maxSources),
		})
		if err != nil {
			return nil, err
		}
		if response == nil {
			return nil, errx.NewWithCode(errx.ServiceUnavailable)
		}

		sources := make([]Source, 0, len(response.Posts))
		for _, post := range response.Posts {
			if post == nil || len(sources) >= maxSources {
				continue
			}
			title := strings.TrimSpace(post.Reason)
			if title == "" {
				title = "Recommended post"
			}
			sources = append(sources, Source{Type: "post", ID: strconv.FormatInt(post.PostId, 10), Title: truncate(title, 120)})
		}
		if len(sources) == 0 {
			return &Result{Text: "There are no recommendations available right now."}, nil
		}
		return &Result{Text: fmt.Sprintf("Found %d recommendations for you.", len(sources)), Sources: sources}, nil
	}
}

func userHandler(client userservice.UserService) handler {
	return func(ctx context.Context, request Request) (*Result, error) {
		if client == nil {
			return nil, errx.NewWithCode(errx.ServiceUnavailable)
		}
		response, err := client.GetUser(ctx, &userservice.GetUserReq{UserId: request.UserID})
		if err != nil {
			return nil, err
		}
		if response == nil || response.User == nil {
			return nil, errx.NewWithCode(errx.UserNotFound)
		}
		profile := response.User
		name := strings.TrimSpace(profile.Nickname)
		if name == "" {
			name = profile.Username
		}
		return &Result{
			Text: fmt.Sprintf("Your profile is %s. You have %d followers and follow %d users.", truncate(name, 120), profile.FollowerCount, profile.FollowingCount),
			Sources: []Source{{
				Type:  "user",
				ID:    strconv.FormatInt(profile.Id, 10),
				Title: truncate(name, 120),
			}},
		}, nil
	}
}

func truncate(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}
