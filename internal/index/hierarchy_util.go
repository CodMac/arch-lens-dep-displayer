package index

import "strings"

// splitQN 返回 QN 的点号逐级前缀序列。
// 例："com.demo.service" → ["com", "com.demo", "com.demo.service"]。
func splitQN(qn string) []string {
	var segs []string
	acc := ""
	for _, p := range strings.Split(qn, ".") {
		if p == "" {
			continue
		}
		if acc == "" {
			acc = p
		} else {
			acc = acc + "." + p
		}
		segs = append(segs, acc)
	}
	return segs
}

// addClassPackage 把类 QN 的包前缀加入包集合，用已知包集合判定边界。
// 对嵌套类/多段 QN（com.demo.OrderService.OrderInner），
// 取已知包集合内最长前缀（com.demo）而非错误地把 OrderService 当包。
// 无已知包时兜底：去掉类名段后逐级拆。
func addClassPackage(set map[string]bool, qn string, explicitPkgs []string) {
	// 去方法签名后，找已知包集合内的最长前缀。
	nosig := qn
	if i := strings.Index(nosig, "("); i >= 0 {
		nosig = nosig[:i]
	}
	best := ""
	for _, p := range explicitPkgs {
		if p != "" && strings.HasPrefix(nosig, p+".") && len(p) > len(best) {
			best = p
		}
	}
	if best != "" {
		for _, seg := range splitQN(best) {
			set[seg] = true
		}
		return
	}
	// 无已知包：去掉类名段（最后一段）后逐级拆。
	if i := strings.LastIndex(nosig, "."); i >= 0 {
		for _, seg := range splitQN(nosig[:i]) {
			set[seg] = true
		}
	}
}

// classPackage 返回类 QN 归属的包（已知包集合内最长前缀，兜底去类名段）。
func classPackage(qn string, explicitPkgs []string) string {
	nosig := qn
	if i := strings.Index(nosig, "("); i >= 0 {
		nosig = nosig[:i]
	}
	best := ""
	for _, p := range explicitPkgs {
		if p != "" && strings.HasPrefix(nosig, p+".") && len(p) > len(best) {
			best = p
		}
	}
	if best != "" {
		return best
	}
	if i := strings.LastIndex(nosig, "."); i >= 0 {
		return nosig[:i]
	}
	return ""
}

// parentPackage 返回包 QN 的父包（去最后一段），顶层返回 ""。
func parentPackage(pkg string) string {
	if i := strings.LastIndex(pkg, "."); i >= 0 {
		return pkg[:i]
	}
	return ""
}

// hostClass 从成员 QN 提取宿主类 QN。
// "com.demo.OrderService.getOrder()" → "com.demo.OrderService"；
// "com.demo.OrderService.field" → "com.demo.OrderService"。
func hostClass(qn string) string {
	nosig := qn
	if i := strings.Index(nosig, "("); i >= 0 {
		nosig = nosig[:i]
	}
	if i := strings.LastIndex(nosig, "."); i >= 0 {
		return nosig[:i]
	}
	return ""
}
