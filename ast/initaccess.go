package ast

/*
test() // delta is uninitialised when test is called
var delta = 10

fn test() {
	echo delta // null
}
*/

// intended to be used the first time a function is called
/* func (cs *compilerState) ensureGlobalAccessIsInitialized(n Node) bool {
	switch node := n.(type) {
	case IdentDec:
		return cs.ensureGlobalAccessIsInitialized(node.Value)
	case IdentGet:
		this := cs.rc
		for scroll := 0; this != nil; scroll++ {
			for i := len(this.lookup) - 1; i >= 0; i-- {
				if _, exists := this.lookup[i][node.Name]; exists {
					// if accessing global; make sure it is initialized
					if this.previous != nil && this.previous.previous == nil {
						if _, has := cs.uninitializedGlobals[node.Name]; has {
							return false
						}
					}
					return true
				}
			}
			this = this.previous
		}
		return true
	case IdentSet:
		this := cs.rc
		for scroll := 0; this != nil; scroll++ {
			for i := len(this.lookup) - 1; i >= 0; i-- {
				if _, exists := this.lookup[i][node.Name]; exists {
					// if accessing global; make sure it is initialized
					if this.previous != nil && this.previous.previous == nil {
						if _, has := cs.uninitializedGlobals[node.Name]; has {
							return false
						}
					}
					return true
				}
			}
			this = this.previous
		}
		return true
	}

	panic("ensureReachability() -> unknown node")
} */
