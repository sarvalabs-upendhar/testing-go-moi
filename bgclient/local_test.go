package bgclient

//
// func TestNewTestCluster(t *testing.T) {
//	l := NewLocalNetwork(DefaultClusterConfig(), nil)
//
//	_, err := l.StartNetwork(context.Background())
//	require.NoError(t, err)
//
//	time.Sleep(5 * time.Second)
//
//	x, err := l.JSONRpcUrls(context.Background())
//	require.NoError(t, err)
//	fmt.Println("json urls", x)
//
//	y, err := l.NetworkStatus(context.Background())
//	require.NoError(t, err)
//
//	fmt.Println("network  status ", y)
//
//	fmt.Println("before stop ", x[0])
//	err = l.StopNode(context.Background(), x[0])
//	require.NoError(t, err)
//
//	fmt.Println("after stop ")
//	fmt.Println("before start ")
//
//	err = l.StartNode(context.Background(), x[0], false)
//	require.NoError(t, err)
//
//	fmt.Println("after start ")
//	time.Sleep(5 * time.Second)
//
//	err = l.DestroyNetwork(context.Background(), true)
//	require.NoError(t, err)
//}
