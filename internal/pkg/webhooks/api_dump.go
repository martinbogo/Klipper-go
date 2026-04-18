package webhooks

import reportpkg "goklipper/internal/pkg/motion/report"

type runtimeAPIDumpClient struct {
	client RuntimeClient
}

func NewAPIDumpClient(client RuntimeClient) reportpkg.APIDumpClient {
	return &runtimeAPIDumpClient{client: client}
}

func (c *runtimeAPIDumpClient) IsClosed() bool {
	return c.client.IsClosed()
}

func (c *runtimeAPIDumpClient) Send(msg map[string]interface{}) {
	c.client.Send(msg)
}
