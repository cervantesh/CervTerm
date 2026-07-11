package glfwgl

type shelf struct {
	y, height, nextX int
}

type shelfPacker struct {
	width, height int
	shelves       []shelf
}

func newShelfPacker(width, height int) shelfPacker {
	return shelfPacker{width: width, height: height}
}

func (p *shelfPacker) Insert(width, height int) (int, int, bool) {
	if width <= 0 || height <= 0 || width > p.width || height > p.height {
		return 0, 0, false
	}
	for i := range p.shelves {
		s := &p.shelves[i]
		if s.height == height && s.nextX+width <= p.width {
			x := s.nextX
			s.nextX += width
			return x, s.y, true
		}
	}
	y := 0
	if len(p.shelves) > 0 {
		last := p.shelves[len(p.shelves)-1]
		y = last.y + last.height
	}
	if y+height > p.height {
		return 0, 0, false
	}
	p.shelves = append(p.shelves, shelf{y: y, height: height, nextX: width})
	return 0, y, true
}

func (p *shelfPacker) Reset() {
	p.shelves = nil
}

func entryGenerationValid(entryGeneration, generation uint64) bool {
	return entryGeneration == generation
}
