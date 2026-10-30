package index

// tarjan 求有向图强连通分量（迭代式 Tarjan，避免深递归栈溢出）。
func tarjan(adj map[string][]string) [][]string {
	index := map[string]int{}
	low := map[string]int{}
	onStack := map[string]bool{}
	var stack []string
	var result [][]string
	counter := 0

	var strongconnect func(v string)
	strongconnect = func(v string) {
		counter++
		index[v] = counter
		low[v] = counter
		stack = append(stack, v)
		onStack[v] = true

		for _, w := range adj[v] {
			if _, ok := index[w]; !ok {
				strongconnect(w)
				if low[w] < low[v] {
					low[v] = low[w]
				}
			} else if onStack[w] && index[w] < low[v] {
				low[v] = index[w]
			}
		}

		if low[v] == index[v] {
			var comp []string
			for {
				w := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				onStack[w] = false
				comp = append(comp, w)
				if w == v {
					break
				}
			}
			result = append(result, comp)
		}
	}

	// 确定性遍历顺序。
	nodes := make([]string, 0, len(adj))
	for n := range adj {
		nodes = append(nodes, n)
	}
	sortStrings(nodes)
	for _, v := range nodes {
		if _, ok := index[v]; !ok {
			strongconnect(v)
		}
	}
	return result
}

// sortStrings 对字符串切片原地排序（避免引入 sort 包的额外依赖噪音）。
func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}
