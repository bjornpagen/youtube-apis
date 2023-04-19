package mediadownloader

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"go.uber.org/ratelimit"
)

type Option func(option *options) error

type options struct {
	host       string
	rateLimit  *ratelimit.Limiter
	httpClient *http.Client
}

func WithHost(host string) Option {
	return func(option *options) error {
		// Check if host is valid.
		_, err := http.NewRequest("GET", fmt.Sprintf("https://%s", host), nil)
		if err != nil {
			return fmt.Errorf("invalid host: %w", err)
		}

		option.host = host
		return nil
	}
}

func WithRateLimit(rl ratelimit.Limiter) Option {
	return func(option *options) error {
		option.rateLimit = &rl
		return nil
	}
}

func WithHttpClient(hc http.Client) Option {
	return func(option *options) error {
		option.httpClient = &hc
		return nil
	}
}

type Client struct {
	apiKey  string
	options *options
}

func New(apiKey string, opts ...Option) (*Client, error) {
	o := &options{}
	for _, opt := range opts {
		err := opt(o)
		if err != nil {
			return nil, fmt.Errorf("bad option: %w", err)
		}
	}

	if o.host == "" {
		o.host = "youtube-media-downloader.p.rapidapi.com"
	}

	if o.rateLimit == nil {
		o.rateLimit = new(ratelimit.Limiter)
		*o.rateLimit = ratelimit.New(3, ratelimit.Per(time.Second))
	}

	if o.httpClient == nil {
		o.httpClient = http.DefaultClient
	}

	return &Client{
		apiKey:  apiKey,
		options: o,
	}, nil
}

type getChannelVideosOption func(option *getChannelVideosOptions) error

type getChannelVideosOptions struct {
	lang        string
	contentType ContentType
}

type ContentType string

const (
	ContentTypeVideos    = ContentType("videos")
	ContentTypeShorts    = ContentType("shorts")
	ContentTypeLive      = ContentType("live")
	ContentTypeUndefined = ContentType("")
)

func WithLang(lang string) getChannelVideosOption {
	return func(option *getChannelVideosOptions) error {
		option.lang = lang
		return nil
	}
}

func WithContentType(contentType ContentType) getChannelVideosOption {
	return func(option *getChannelVideosOptions) error {
		option.contentType = contentType
		return nil
	}
}

type getChannelVideosResponse struct {
	Status    bool    `json:"status"`
	NextToken string  `json:"nextToken"`
	Items     []Video `json:"items"`
}

type Video struct {
	Type              string      `json:"type"`
	ID                string      `json:"id"`
	Title             string      `json:"title"`
	IsLiveNow         bool        `json:"isLiveNow"`
	LengthText        string      `json:"lengthText"`
	ViewCountText     string      `json:"viewCountText"`
	PublishedTimeText string      `json:"publishedTimeText"`
	Thumbnails        []Thumbnail `json:"thumbnails"`
}

type Thumbnail struct {
	URL    string `json:"url"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
	Moving bool   `json:"moving"`
}

func (c *Client) GetChannelVideos(channelID string, opts ...getChannelVideosOption) ([]Video, error) {
	o := &getChannelVideosOptions{}
	for _, opt := range opts {
		err := opt(o)
		if err != nil {
			return nil, fmt.Errorf("bad option: %w", err)
		}
	}

	if o.lang == "" {
		o.lang = "en"
	}
	if o.contentType == ContentTypeUndefined {
		o.contentType = ContentTypeVideos
	}

	url := fmt.Sprintf("https://%s/v2/channel/videos?channelId=%s&lang=%s", c.options.host, channelID, o.lang)

	(*c.options.rateLimit).Take()
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Add("X-RapidAPI-Key", c.apiKey)
	req.Header.Add("X-RapidAPI-Host", c.options.host)

	res, err := c.options.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("http status code is not ok: %s", string(body))
	}

	response := &getChannelVideosResponse{}
	err = json.Unmarshal(body, response)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal response body: %w", err)
	}

	return response.Items, nil
}
