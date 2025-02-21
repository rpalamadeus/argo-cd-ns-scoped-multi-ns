package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"sync"
	"time"

	"github.com/golang/snappy"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

var (
	ctx              = context.Background()
	useExternalCache bool
	useCompression   bool
	namespace        string
	proxyURL         string
	rdb              *redis.Client
	inMemoryCache    sync.Map

	// Prometheus Metrics
	cacheHits = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "cache_hits_total",
		Help: "Total number of cache hits",
	})
	cacheMisses = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "cache_misses_total",
		Help: "Total number of cache misses",
	})
	cacheStores = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "cache_store_total",
		Help: "Total number of cache store operations",
	})
	cacheDeletes = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "cache_delete_total",
		Help: "Total number of cache delete operations",
	})
	compressionDuration = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "compression_duration_seconds",
		Help:    "Duration of snappy compression in seconds",
		Buckets: prometheus.DefBuckets,
	})
	decompressionDuration = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "decompression_duration_seconds",
		Help:    "Duration of snappy decompression in seconds",
		Buckets: prometheus.DefBuckets,
	})
	goCollector      = prometheus.NewGoCollector()
	processCollector = prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{})
)

func init() {
	//prometheus.MustRegister(cacheHits, cacheMisses, cacheStores, cacheDeletes, compressionDuration, decompressionDuration, goCollector, processCollector)
}
func logCPUAndMemoryUsage() {
	ticker := time.NewTicker(10 * time.Second)
	for range ticker.C {
		var memStats runtime.MemStats
		runtime.ReadMemStats(&memStats)

		fmt.Printf("\n[Metrics] Time: %s\n", time.Now().Format(time.RFC3339))
		fmt.Printf("[Metrics] CPU: Goroutines: %d\n", runtime.NumGoroutine())
		fmt.Printf("[Metrics] Memory: Alloc = %v MB, Sys = %v MB, HeapAlloc = %v MB, HeapSys = %v MB\n",
			memStats.Alloc/1024/1024,
			memStats.Sys/1024/1024,
			memStats.HeapAlloc/1024/1024,
			memStats.HeapSys/1024/1024,
		)

		// Optional: Save CPU profile to file
		f, err := os.Create("cpu_profile.pprof")
		if err == nil {
			pprof.Lookup("goroutine").WriteTo(f, 0)
			f.Close()
		}
	}
}
func newRedisClient() *redis.Client {
	return redis.NewClient(&redis.Options{
		Addr:     "localhost:6379",
		Password: "",
		DB:       0,
	})
}

func newK8sClient(proxyURL string) (*kubernetes.Clientset, error) {
	kubeconfig := filepath.Join(homedir.HomeDir(), ".kube", "config")
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return nil, err
	}
	if proxyURL != "" {
		proxy, err := url.Parse(proxyURL)
		if err != nil {
			return nil, fmt.Errorf("invalid proxy URL: %v", err)
		}
		config.Transport = &http.Transport{
			Proxy: http.ProxyURL(proxy),
		}
		fmt.Printf("Using HTTP proxy: %s\n", proxyURL)
	}

	return kubernetes.NewForConfig(config)
}

func maybeCompress(data []byte) []byte {
	if useCompression {
		start := time.Now()
		compressedData := snappy.Encode(nil, data)
		compressionDuration.Observe(time.Since(start).Seconds())
		return compressedData
	}
	return data
}

func maybeDecompress(data []byte) ([]byte, error) {
	if useCompression {
		start := time.Now()
		decompressedData, err := snappy.Decode(nil, data)
		decompressionDuration.Observe(time.Since(start).Seconds())
		return decompressedData, err
	}
	return data, nil
}

func cachePod(pod *v1.Pod) error {
	key := fmt.Sprintf("pod:%s:%s", pod.Namespace, pod.Name)

	data, err := json.Marshal(pod)
	if err != nil {
		return err
	}
	data = maybeCompress(data)

	if useExternalCache {
		err = rdb.Set(ctx, key, data, time.Hour).Err()
	} else {
		inMemoryCache.Store(key, data)
	}
	cacheStores.Inc()
	return err
}

func removePod(pod *v1.Pod) error {
	key := fmt.Sprintf("pod:%s:%s", pod.Namespace, pod.Name)

	var err error
	if useExternalCache {
		err = rdb.Del(ctx, key).Err()
	} else {
		inMemoryCache.Delete(key)
	}
	cacheDeletes.Inc()
	return err
}

func listPods() ([]*v1.Pod, error) {
	pods := []*v1.Pod{}

	if useExternalCache {
		keys, err := rdb.Keys(ctx, "pod:*").Result()
		if err != nil {
			return nil, err
		}

		for _, key := range keys {
			data, err := rdb.Get(ctx, key).Bytes()
			if err != nil {
				cacheMisses.Inc()
				continue
			}
			cacheHits.Inc()

			data, err = maybeDecompress(data)
			if err != nil {
				return nil, err
			}

			var pod v1.Pod
			if err := json.Unmarshal(data, &pod); err != nil {
				return nil, err
			}
			pods = append(pods, &pod)
		}
	} else {
		inMemoryCache.Range(func(key, value interface{}) bool {
			data, _ := value.([]byte)
			cacheHits.Inc()

			data, _ = maybeDecompress(data)
			var pod v1.Pod
			_ = json.Unmarshal(data, &pod)
			pods = append(pods, &pod)
			return true
		})
	}
	return pods, nil
}

func startMetricsServer() {
	http.Handle("/metrics", promhttp.Handler())
	fmt.Println("Serving metrics at :8080/metrics")
	go http.ListenAndServe(":8080", nil)
}

func watchPods(clientset *kubernetes.Clientset) {
	fmt.Printf("Watching pods in namespace: %s\n", namespace)

	lw := cache.NewListWatchFromClient(
		clientset.CoreV1().RESTClient(),
		"pods",
		namespace,
		fields.Everything(),
		//metav1.ListOptions{},
	)

	_, controller := cache.NewInformer(lw, &v1.Pod{}, 0,
		cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				pod := obj.(*v1.Pod)
				_ = cachePod(pod)
				fmt.Println("Added Pod + " + pod.Name)
			},
			UpdateFunc: func(oldObj, newObj interface{}) {
				pod := newObj.(*v1.Pod)
				_ = cachePod(pod)
				fmt.Println("Updated Pod + " + pod.Name)
			},
			DeleteFunc: func(obj interface{}) {
				pod := obj.(*v1.Pod)
				_ = removePod(pod)
				fmt.Println("Deleted Pod + " + pod.Name)
			},
		})

	stop := make(chan struct{})
	go controller.Run(stop)
}

func main() {
	flag.BoolVar(&useExternalCache, "external-cache", false, "Use external Redis cache instead of in-memory cache")
	flag.BoolVar(&useCompression, "compress", false, "Enable Snappy compression for cached data")
	flag.StringVar(&namespace, "namespace", "default", "Kubernetes namespace to watch for pods")
	flag.StringVar(&proxyURL, "proxy", "", "Kubernetes client proxy url")
	flag.Parse()

	if useExternalCache {
		rdb = newRedisClient()
		defer rdb.Close()
	}

	clientset, _ := newK8sClient(proxyURL)
	go logCPUAndMemoryUsage()

	startMetricsServer()

	watchPods(clientset)
	select {}
}
