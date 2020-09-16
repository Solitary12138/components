package memory_cache

import (
	"errors"
	"sync"
	"time"
)

const (
	defultInterval   time.Duration = time.Second * 1  //默认间隔
	defultExpiration time.Duration = time.Duration(0) //默认不过期
	defultKeyLen     int8          = 3                //默认key分割长度
)

type item struct {
	object     interface{} //value
	expiration int64       //过期时间
}

type janitor struct {
	stop     chan bool     //停止
	interval time.Duration //间隔时间
}

func (this *janitor) run(c *Cache) {
	ticker := time.NewTicker(this.interval)
	for {
		now := time.Now().UnixNano()
		select {
		case <-ticker.C:
			c.mu.Lock()
			for k, v := range c.items {
				if v.expiration > 0 && v.expiration < now {
					delete(c.items, k)
				}
			}
			c.mu.Unlock()
		}
	}
}
func newJanitor(d time.Duration) *janitor {
	if d <= 0 {
		d = defultInterval
	}
	return &janitor{
		stop:     make(chan bool),
		interval: d,
	}
}

type Cache struct {
	items      map[string]item //缓存数据
	mu         sync.RWMutex    //读写锁
	janitor    *janitor        //定时删除过期缓存
	expiration int64           //整个缓存过期时间
}

func newCache(expiration time.Duration, interval time.Duration) *Cache {
	c := &Cache{
		items:   make(map[string]item),
		janitor: newJanitor(interval),
	}
	go c.janitor.run(c)
	if expiration <= 0 {
		c.expiration = 0
	} else {
		c.expiration = time.Now().Add(expiration).UnixNano()
	}
	return c
}

func (this *Cache) getValue(key string) (interface{}, bool) {
	now := time.Now().UnixNano()
	this.mu.RLock()
	v, ok := this.items[key]
	this.mu.RUnlock()
	if ok {
		if v.expiration > 0 && now > v.expiration {
			return nil, false
		}
		return v.object, true
	}
	return nil, false
}

func (this *Cache) setValue(key string, value interface{}, expiration time.Duration) {
	this.mu.Lock()
	v := item{
		object: value,
	}
	if expiration > 0 {
		v.expiration = time.Now().Add(expiration).UnixNano()
	}
	this.items[key] = v
	this.mu.Unlock()
}

type CacheGroup struct {
	caches     map[string]*Cache //缓存组
	expiration int64             //整个缓存过期时间
	mu         sync.RWMutex      //锁 用于新增和删除map
	keyLen     int8
}

func NewCacheGroup(expiration time.Duration) *CacheGroup {
	return &CacheGroup{
		caches:     make(map[string]*Cache),
		expiration: time.Now().Add(expiration).UnixNano(),
		keyLen:     defultKeyLen,
	}
}

func (this *CacheGroup) addCache(key string, expiration time.Duration, interval time.Duration) {
	this.mu.Lock()
	key = this.getSplitKey(key)
	if _, ok := this.caches[key]; ok {
		this.mu.Unlock()
		return
	}
	this.caches[key] = newCache(expiration, interval)
	this.mu.Unlock()
}

func (this *CacheGroup) addDefultCache(key string) *Cache {
	this.mu.Lock()
	key = this.getSplitKey(key)
	if v, ok := this.caches[key]; ok {
		this.mu.Unlock()
		return v
	}
	c := newCache(defultExpiration, defultInterval)
	this.caches[key] = c
	//fmt.Println(key)
	this.mu.Unlock()
	return c
}

func (this *CacheGroup) getCache(key string) (*Cache, bool) {
	this.mu.RLock()
	key = this.getSplitKey(key)
	v, ok := this.caches[key]
	this.mu.RUnlock()
	return v, ok
}

func (this *CacheGroup) GetValue(key string) (interface{}, bool) {
	if c, ok := this.getCache(key); ok {
		return c.getValue(key)
	}
	return nil, false
}

func (this *CacheGroup) GetString(key string) (string, error) {
	if v, ok := this.GetValue(key); ok {
		if sv, ok := v.(string); ok {
			return sv, nil
		} else {
			return "", errors.New("value not is string")
		}
	}
	return "", errors.New("key not found")
}

func (this *CacheGroup) SetValue(key string, value interface{}, expiration time.Duration) {
	c, ok := this.getCache(key)
	if !ok {
		c = this.addDefultCache(key)
		//fmt.Println("errorss")
	}
	c.setValue(key, value, expiration)
}

func (this *CacheGroup) deleteCache(key string) error {
	this.mu.Lock()
	key = this.getSplitKey(key)
	if _, ok := this.caches[key]; !ok {
		this.mu.Unlock()
		return errors.New("key not exist")
	}
	delete(this.caches, key)
	this.mu.Unlock()
	return nil
}

func (this *CacheGroup) getSplitKey(key string) string {
	if len(key) <= int(this.keyLen) {
		return key
	}
	return string(key[:this.keyLen])
}
