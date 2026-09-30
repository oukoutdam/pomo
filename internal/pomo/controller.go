package pomo

import "time"

type Controller struct {
	Engine    *Engine
	Store     *Store
	Config    Config
	Completed int
	LongBreak bool
}

func NewController(store *Store, config Config, kind string, duration time.Duration, now time.Time) (*Controller, error) {
	c := &Controller{Engine: NewEngine(kind, duration, now), Store: store, Config: config}
	return c, store.Save(&c.Engine.Session)
}

func (c *Controller) Advance(now time.Time) (bool, bool) {
	completed, slept := c.Engine.Advance(now)
	if completed && c.Engine.Session.Kind == "focus" {
		c.Completed++
	}
	return completed, slept
}

func (c *Controller) Save() error { return c.Store.Save(&c.Engine.Session) }

func (c *Controller) Stop() error {
	c.Engine.Finish()
	return c.Save()
}

func (c *Controller) StartBreak(now time.Time) error {
	if c.Engine.Session.Kind != "focus" {
		return nil
	}
	duration, long := nextBreak(c.Config, c.Completed, c.Engine.Session.Completed)
	if err := c.Stop(); err != nil {
		return err
	}
	c.Engine = NewEngine("break", duration, now)
	c.LongBreak = long
	return c.Save()
}

func (c *Controller) StartFocus(now time.Time) error {
	if c.Engine.Session.Kind != "break" {
		return nil
	}
	if err := c.Stop(); err != nil {
		return err
	}
	duration, _ := parseDuration(c.Config.Focus)
	c.Engine = NewEngine("focus", duration, now)
	c.LongBreak = false
	return c.Save()
}
