package slicecache

import (
	"sync"
)

//这种数据结构适合于在一个大流程中，需要计算很多步，用普通切片会频繁申请内存的情况,暂时只支持int类型
//注意:
//1)非并发安全，需要自行控制锁的应用，一般是让其在单线程内运行;
//2)开始新的计算时，需要调用clearnData初始化数据(或者在确保前面的计算结果已经无人会再使用时);
//3)合理设定缓存切片cache

//总切片中，子切片的开始结束位置
type ChildSliceIndex int
type index struct {
	begin int
	end   int
}
type SliceCache struct {
	Locker           *sync.Mutex
	lenOfCache       int     //初始切片长度
	curLenOfCache    int     //可能发生扩容后的实际缓存长度
	notUsedDataBegin int     //data中未使用的数据开始位置
	childSliceNum    int     //存储在data中子切片数量
	childSliceIndex  []index //所有存储在data中子切片下标开始结束位置
	cache            []int   //用于存储子切片
}

//lenOfCache指总的分配的空间长度，avgLenOfChildSlice为可先参数，需要根据实际情况设置合理的长度默认长度为10
func New(lenOfCache int) *SliceCache {
	if lenOfCache < 1024 {
		lenOfCache = 1024
	}
	s := new(SliceCache)
	s.lenOfCache = lenOfCache
	s.curLenOfCache = lenOfCache
	s.cache = make([]int, lenOfCache)
	s.childSliceIndex = make([]index, lenOfCache) //注意，实际上索引的个数应该是要远低于lenOfCache，但在此，为了简单起见，暂时先浪费点空间
	s.Locker = new(sync.Mutex)
	return s
}

//清空数据，如果未发生过扩容，则把相关数据置零即可，否则，需要重新初始化部分字段
func (this *SliceCache) ClearnData() {
	if this.lenOfCache == this.curLenOfCache {
		this.notUsedDataBegin = 0
		this.childSliceNum = 0
	} else {
		this.curLenOfCache = this.lenOfCache
		this.cache = make([]int, this.lenOfCache)
		this.childSliceIndex = make([]index, this.lenOfCache)
		this.notUsedDataBegin = 0
		this.childSliceNum = 0
	}
}

//从外部添加数据，并返回一个内部编号
func (this *SliceCache) AppendFromOutside(data ...int) (childSliceIndex ChildSliceIndex) {
	if len(data) == 0 {
		panic("no cache")
	}
	if this.notUsedDataLen() < len(data) {
		this.growslice()
	}
	begin := this.notUsedDataBegin
	this.notUsedDataBegin = this.notUsedDataBegin + len(data)
	end := this.notUsedDataBegin
	copy(this.cache[begin:end], data)
	this.childSliceIndex[this.childSliceNum] = index{begin, end}
	this.childSliceNum = this.childSliceNum + 1
	return ChildSliceIndex(this.childSliceNum - 1)
}

//从自身添加
func (this *SliceCache) Append(oldchildSliceIndex ChildSliceIndex, data ...int) (newchildSliceIndex ChildSliceIndex) {
	if len(data) == 0 {
		panic("no cache")
	}
	var begin, end int
	//先添加原有的
	//如果老数据已经是切片的最后一条有效数据，那么不必再复制该数据，直接在原有数据基础上增加数据即可
	if this.notUsedDataBegin == this.childSliceIndex[oldchildSliceIndex].end {
		if this.notUsedDataLen() < len(data) {
			this.growslice()
		}
		begin = this.childSliceIndex[oldchildSliceIndex].begin
		end = this.childSliceIndex[oldchildSliceIndex].end
	} else {
		if this.notUsedDataLen() < len(data)+this.LenOf(oldchildSliceIndex) {
			this.growslice()
		}
		begin = this.notUsedDataBegin
		oldDataLen := this.childSliceIndex[oldchildSliceIndex].end - this.childSliceIndex[oldchildSliceIndex].begin
		this.notUsedDataBegin = this.notUsedDataBegin + oldDataLen
		end = this.notUsedDataBegin
		copy(this.cache[begin:end], this.cache[this.childSliceIndex[oldchildSliceIndex].begin:this.childSliceIndex[oldchildSliceIndex].end])
	}
	//再添加新增的
	tempBegin := this.notUsedDataBegin
	this.notUsedDataBegin = this.notUsedDataBegin + len(data)
	end = this.notUsedDataBegin
	copy(this.cache[tempBegin:end], data)
	//处理索引
	this.childSliceIndex[this.childSliceNum] = index{begin, end}
	this.childSliceNum = this.childSliceNum + 1
	return ChildSliceIndex(this.childSliceNum - 1)
}

//数据长度
func (this *SliceCache) LenOf(childSliceIndex ChildSliceIndex) int {
	return this.childSliceIndex[childSliceIndex].end - this.childSliceIndex[childSliceIndex].begin
}

//获取某位置的所有元素
func (this *SliceCache) ToSlice(childSliceIndex ChildSliceIndex) (ret []int) {
	ret = make([]int, this.childSliceIndex[childSliceIndex].end-this.childSliceIndex[childSliceIndex].begin)
	copy(ret, this.cache[this.childSliceIndex[childSliceIndex].begin:this.childSliceIndex[childSliceIndex].end])
	return
}

//获取某位置的第一个元素
func (this *SliceCache) GetFirstElemOf(childSliceIndex ChildSliceIndex) int {
	return this.cache[this.childSliceIndex[childSliceIndex].begin]
}

//获取某位置的最后一个元素
func (this *SliceCache) GetLastElemOf(childSliceIndex ChildSliceIndex) int {
	return this.cache[this.childSliceIndex[childSliceIndex].begin+this.childSliceIndex[childSliceIndex].end-this.childSliceIndex[childSliceIndex].begin-1]
}

//未使用的数据长度
func (this *SliceCache) notUsedDataLen() int {
	return this.curLenOfCache - this.notUsedDataBegin
}

//扩容 为简单起见直接按翻倍来处理
func (this *SliceCache) growslice() {
	newChildSliceIndex := make([]index, this.curLenOfCache*2)
	newCache := make([]int, this.curLenOfCache*2)
	copy(newChildSliceIndex, this.childSliceIndex)
	copy(newCache, this.cache)
	this.childSliceIndex = newChildSliceIndex
	this.cache = newCache
	this.curLenOfCache = this.curLenOfCache * 2
}
