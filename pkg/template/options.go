package template

const (
	defaultAddr = "127.0.0.1"
	defaultPort = 2379
)

type Option func(*Options)

type Options struct {
	Address string
	Port    int
}

func (o *Options) Apply(options ...Option) {
	for _, option := range options {
		option(o)
	}
	if o.Address == "" {
		o.Address = defaultAddr
	}
	if o.Port == 0 {
		o.Port = defaultPort
	}
}

func WithAddress(addr string) Option {
	return func(o *Options) {
		o.Address = addr
	}
}

func WithPort(port int) Option {
	return func(o *Options) {
		o.Port = port
	}
}
