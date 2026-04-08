package bgclient

// func defaultConfig() *Config {
//	return &Config{
//		Network:     CLOUD,
//		CloudCfg:    server.DefaultCloudConfig(),
//		EndPoint:    "http://85.239.245.54:7000/api",
//		DialTimeout: 10 * time.Second,
//	}
//}
//
// func TestClient_Ping(t *testing.T) {
//	ctx := context.Background()
//	client := NewClient(defaultConfig())
//	err := client.ServerStatus(ctx)
//	log.Println(err)
//}
//
// func TestClient_Status(t *testing.T) {
//	ctx := context.Background()
//	client := NewClient(defaultConfig())
//	status, err := client.NetworkStatus(ctx)
//	log.Println(status, err)
//}
//
// func TestClient_Accounts(t *testing.T) {
//	ctx := context.Background()
//	client := NewClient(defaultConfig())
//	accounts, err := client.Accounts(ctx)
//	log.Println(accounts, err)
//}
//
// func TestClient_JSONRpcUrls(t *testing.T) {
//	ctx := context.Background()
//	client := NewClient(defaultConfig())
//	urls, err := client.JSONRpcUrls(ctx)
//	log.Println(urls, err)
//}
//
// func TestClient_Deploy(t *testing.T) {
//	ctx := context.Background()
//	client := NewClient(defaultConfig())
//
//	accounts, err := client.StartNetwork(ctx)
//
//	log.Println(accounts, err)
//}
//
// func TestClient_Stop(t *testing.T) {
//	ctx := context.Background()
//	client := NewClient(defaultConfig())
//
//	err := client.DestroyNetwork(ctx, false)
//
//	log.Println(err)
//}
