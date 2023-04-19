package yttranscriptor

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
		o.host = "youtube-transcriptor.p.rapidapi.com"
	}

	if o.rateLimit == nil {
		o.rateLimit = new(ratelimit.Limiter)
		*o.rateLimit = ratelimit.New(10, ratelimit.Per(time.Second))
	}

	if o.httpClient == nil {
		o.httpClient = http.DefaultClient
	}

	return &Client{
		apiKey:  apiKey,
		options: o,
	}, nil
}

type getTranscriptOption func(option *getTranscriptOptions) error

type getTranscriptOptions struct {
	lang string
}

func WithLang(lang string) getTranscriptOption {
	return func(option *getTranscriptOptions) error {
		option.lang = lang
		return nil
	}
}

type GetTranscriptResponse struct {
	Title           string          `json:"title"`
	Description     string          `json:"description"`
	AvailableLangs  []string        `json:"availableLangs"`
	LengthInSeconds string          `json:"lengthInSeconds"`
	Thumbnails      []Thumbnail     `json:"thumbnails"`
	Transcription   []Transcription `json:"transcription"`
}

// GetTranscriptResponse.String()
func (g *GetTranscriptResponse) String() string {
	// just concatenate all subtitles
	var subtitles string
	for _, t := range g.Transcription {
		subtitles += t.Subtitle + " "
	}

	if len(subtitles) == 0 {
		return ""
	}

	return subtitles[:len(subtitles)-1]
}

type Thumbnail struct {
	URL    string `json:"url"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

type Transcription struct {
	Subtitle string  `json:"subtitle"`
	Start    float64 `json:"start"`
	Dur      float64 `json:"dur"`
}

func (c *Client) GetTranscript(videoID string, opts ...getTranscriptOption) (*GetTranscriptResponse, error) {
	o := &getTranscriptOptions{}
	for _, opt := range opts {
		err := opt(o)
		if err != nil {
			return nil, fmt.Errorf("bad option: %w", err)
		}
	}

	if o.lang == "" {
		o.lang = "en"
	}

	url := fmt.Sprintf("https://%s/transcript?video_id=%s&lang=%s", c.options.host, videoID, o.lang)

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

	var transcript GetTranscriptResponse
	err = json.Unmarshal(body, &transcript)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &transcript, nil
}
