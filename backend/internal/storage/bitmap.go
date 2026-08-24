package storage

type Bitmap struct {
	bits []byte
	n    int
}

func NewBitmap(n int) *Bitmap {
	return &Bitmap{bits: make([]byte, (n+7)/8), n: n}
}

func (b *Bitmap) Len() int { return b.n }

func (b *Bitmap) Set(i int) {
	b.grow(i + 1)
	b.bits[i/8] |= 1 << (uint(i) % 8)
}

func (b *Bitmap) Clear(i int) {
	b.grow(i + 1)
	b.bits[i/8] &^= 1 << (uint(i) % 8)
}

func (b *Bitmap) Get(i int) bool {
	if i < 0 || i >= b.n {
		return false
	}
	return b.bits[i/8]&(1<<(uint(i)%8)) != 0
}

func (b *Bitmap) grow(n int) {
	if n <= b.n {
		return
	}
	need := (n + 7) / 8
	if need > len(b.bits) {
		nb := make([]byte, need)
		copy(nb, b.bits)
		b.bits = nb
	}
	b.n = n
}

func (b *Bitmap) Append(null bool) {
	i := b.n
	b.grow(i + 1)
	if null {
		b.Set(i)
	}
}

func (b *Bitmap) Count() int {
	c := 0
	for i := 0; i < b.n; i++ {
		if b.Get(i) {
			c++
		}
	}
	return c
}

func (b *Bitmap) Bytes() []byte { return b.bits }

func (b *Bitmap) CloneRange(sel []int) *Bitmap {
	out := NewBitmap(len(sel))
	for i, src := range sel {
		if b.Get(src) {
			out.Set(i)
		}
	}
	return out
}

func BitmapFromBytes(raw []byte, n int) *Bitmap {
	cp := make([]byte, len(raw))
	copy(cp, raw)
	return &Bitmap{bits: cp, n: n}
}
