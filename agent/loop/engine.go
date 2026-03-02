package loop

// Engine drives task execution and emits events.
type Engine struct{}

// executor is a pluggable runner so the loop package can stay cycle-free.
var executor = struct {
	Run func(task Task, emit func(Event)) error
}{
	Run: func(task Task, emit func(Event)) error {
		if emit != nil {
			emit(Event{Type: "agent_reply", Message: "Executed: " + task.Description})
		}
		return nil
	},
}

func SetExecutorRun(run func(task Task, emit func(Event)) error) {
	if run == nil {
		return
	}
	executor.Run = run
}

func NewEngine() *Engine {
	return &Engine{}
}

func (e *Engine) Run(task Task) ([]Event, error) {
	events := make([]Event, 0, 16)
	err := e.RunStream(task, func(ev Event) {
		events = append(events, ev)
	})
	return events, err
}

// RunStream executes one task and emits events incrementally.
func (e *Engine) RunStream(task Task, emit func(Event)) error {
	if emit == nil {
		emit = func(Event) {}
	}
	return executor.Run(task, emit)
}
