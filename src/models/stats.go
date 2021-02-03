package models

type Stats struct {
	Name  string `json:"name"`
	Value int64  `json:"value"`
}

func NewStats(name string) (*Stats, error) {
	c := Stats{Name: name}

	has, err := DB["rdb"].Where("name=?", c.Name).Get(&c)
	if err != nil {
		return nil, err
	}

	if !has {
		c.Save()
	}

	return &c, nil
}

func MustNewStats(name string) *Stats {
	m, err := NewStats(name)
	if err != nil {
		panic(err)
	}

	return m
}

func (p *Stats) Get() int64 {
	has, _ := DB["rdb"].Where("name=?", p.Name).Get(p)
	if !has {
		p.Save()
	}
	return p.Value
}

func (p *Stats) Save() error {
	_, err := DB["rdb"].Insert(p)
	return err
}

func (p *Stats) Del() error {
	_, err := DB["rdb"].Where("name=?", p.Name).Delete(p)
	return err
}

// for GAUAGE
func (p *Stats) Update() error {
	_, err := DB["rdb"].Where("name=?", p.Name).Cols("value").Update(p)
	return err
}

// for COUNTER
func (p *Stats) Inc(i int) error {
	_, err := DB["rdb"].Exec("update from stats set value = value + ? where name=?", i, p.Name)
	return err
}
