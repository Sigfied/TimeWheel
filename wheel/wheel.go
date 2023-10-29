package wheel

import (
	"container/list"
	"log"
	"sync"
	"time"
)

// 封装了一笔定时任务的明细信息
type taskElement struct {
	// 内聚了定时任务执行逻辑的闭包函数
	task func()
	// 定时任务挂载在环状数组中的索引位置
	pos int
	// 定时任务的延迟轮次. 指的是 curSlot 指针还要扫描过环状数组多少轮，才满足执行该任务的条件
	cycle int
	// 定时任务的唯一标识键
	key string
}

type TimeWheel struct {
	sync.Once
	interval time.Duration
	ticker   *time.Ticker // 定时器
	stop     chan struct{}
	// 通过 list 组成的环状数组. 通过遍历环状数组的方式实现时间轮
	// 定时任务数量较大，每个 slot 槽内可能存在多个定时任务，因此通过 list 进行组装
	slots   []*list.List
	current int                      // 当前指针
	kt      map[string]*list.Element // key -> *list.Element
	addChan chan *taskElement        // 添加任务的channel
	delChan chan string              // 删除任务的channel
}

// NewTimeWheel 创建一个时间轮
func NewTimeWheel(slotSize int, interval time.Duration) *TimeWheel {
	if slotSize < 8 {
		slotSize = 8
	}
	if interval <= 0 {
		// 默认时间轮的最小时间粒度为 1s
		interval = time.Second
	}

	t := &TimeWheel{
		interval: interval,
		ticker:   time.NewTicker(interval),
		stop:     make(chan struct{}),
		slots:    make([]*list.List, 0),
		kt:       make(map[string]*list.Element),
		addChan:  make(chan *taskElement),
		delChan:  make(chan string),
	}
	for i := 0; i < slotSize; i++ {
		//初始化槽
		t.slots = append(t.slots, list.New())
	}
	go t.run()
	return t
}

func (tw *TimeWheel) run() {
	defer func() {
		if err := recover(); err != nil {
			log.Printf("time wheel panic: %v\n", err)
		}
	}()
	for {
		select {
		case <-tw.stop:
			return
		case <-tw.ticker.C:
			tw.tick()
		case task := <-tw.addChan:
			tw.addTask(task)
		case rk := <-tw.delChan:
			tw.removeTask(rk)
		}
	}
}

// Stop 停止时间轮
func (tw *TimeWheel) Stop() {
	// 通过单例工具，保证 channel 只能被关闭一次，避免 panic
	tw.Do(func() {
		// 定制定时器 ticker
		tw.ticker.Stop()
		// 关闭定时器运行的 stopc
		close(tw.stop)
	})
}

func (tw *TimeWheel) tick() {
	l := tw.slots[tw.current]
	defer tw.circularIncr()
	tw.execute(l)
}

// 每次 tick 后需要推进 curSlot 指针的位置，slots 在逻辑意义上是环状数组，所以在到达尾部时需要从新回到头部
func (tw *TimeWheel) circularIncr() {
	tw.current = (tw.current + 1) % len(tw.slots)
}

func (tw *TimeWheel) AddTask(key string, delay time.Time, task func()) {
	pos, cycle := tw.getPositionAndCycle(delay)
	log.Printf("add task: %s, pos: %d, cycle: %d\n", key, pos, cycle)
	tw.addChan <- &taskElement{
		task:  task,
		pos:   pos,
		cycle: cycle,
		key:   key,
	}
}

func (tw *TimeWheel) getPositionAndCycle(delay time.Time) (int, int) {
	d := int(time.Until(delay))

	c := d / (len(tw.slots) * int(tw.interval))

	pos := (tw.current + d/int(tw.interval)) % len(tw.slots)
	return pos, c
}

// 常驻 goroutine 接收到创建定时任务后的处理逻辑
func (tw *TimeWheel) addTask(task *taskElement) {
	// 获取到定时任务从属的环状数组 index 以及对应的 l
	l := tw.slots[task.pos]
	// 倘若定时任务 key 之前已存在，则需要先删除定时任务
	if _, ok := tw.kt[task.key]; ok {
		tw.removeTask(task.key)
	}
	if l != nil {
		// 将定时任务追加到 l 尾部
		eTask := l.PushBack(task)
		// 建立定时任务 key 到将定时任务所处的节点
		tw.kt[task.key] = eTask
	}
}

func (tw *TimeWheel) RemoveTask(key string) {
	tw.delChan <- key
}

func (tw *TimeWheel) removeTask(key string) {
	if eTask, ok := tw.kt[key]; ok {
		l := tw.slots[eTask.Value.(*taskElement).pos]
		l.Remove(eTask)
		delete(tw.kt, key)
	}
}

func (tw *TimeWheel) execute(l *list.List) {
	for e := l.Front(); e != nil; {
		task := e.Value.(*taskElement)
		if task.cycle > 0 {
			task.cycle--
			e = e.Next()
			continue
		}
		go func() {
			defer func() {
				if err := recover(); err != nil {
					log.Printf("task panic: %v\n", err)
				}
			}()
			task.task()
		}()
		next := e.Next()
		l.Remove(e)
		delete(tw.kt, task.key)
		e = next
	}
}
