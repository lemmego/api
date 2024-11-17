package event

import (
	"fmt"
)

// Observer interface defines the update method which will be called when the subject changes
type Observer interface {
	Update(message string)
}

// Subject interface defines methods for managing observers
type Subject interface {
	Register(observer Observer)
	Unregister(observer Observer)
	NotifyObservers()
}

// ConcreteSubject stores state of interest to ConcreteObserver objects
// and sends notifications when its state changes
type ConcreteSubject struct {
	observers  []Observer
	state      string
	notifyChan chan string
}

// Register adds an observer to the subject
func (s *ConcreteSubject) Register(observer Observer) {
	s.observers = append(s.observers, observer)
}

// Unregister removes an observer from the subject
func (s *ConcreteSubject) Unregister(observer Observer) {
	for i, obs := range s.observers {
		if obs == observer {
			s.observers = append(s.observers[:i], s.observers[i+1:]...)
			break
		}
	}
}

// NotifyObservers notifies all registered observers about the change
func (s *ConcreteSubject) NotifyObservers() {
	for _, observer := range s.observers {
		observer.Update(s.state)
	}
}

// SetState changes the subject's state and notifies observers
func (s *ConcreteSubject) SetState(state string) {
	s.state = state
	go s.NotifyObservers() // Assuming notification can be asynchronous
}

// ConcreteObserver implements the Observer interface
type ConcreteObserver struct {
	name string
}

// Update receives updates from the subject
func (o *ConcreteObserver) Update(message string) {
	fmt.Printf("%s received: %s\n", o.name, message)
}

func main() {
	// Create subject
	subject := &ConcreteSubject{
		notifyChan: make(chan string),
	}

	// Create observers
	observer1 := &ConcreteObserver{name: "Observer1"}
	observer2 := &ConcreteObserver{name: "Observer2"}

	// Register observers
	subject.Register(observer1)
	subject.Register(observer2)

	// Change subject state which should trigger notifications
	subject.SetState("New State")

	// Unregister an observer
	subject.Unregister(observer1)

	// Change state again
	subject.SetState("Another New State")

	// Allow time for go routines to execute if you're running this in a synchronous context like main
	<-make(chan struct{})
}
