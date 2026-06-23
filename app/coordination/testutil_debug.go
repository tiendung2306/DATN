package coordination

func (s *MockStorage) GetAllApplicationEvents() []*ApplicationEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	var list []*ApplicationEvent
	for _, ev := range s.applicationEvents {
		cp := *ev
		list = append(list, &cp)
	}
	return list
}
