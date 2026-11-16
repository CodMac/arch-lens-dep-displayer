package repository

// ClearCycles 清空某维度的环组/成员/边。
func (r *Repository) ClearCycles(dimID int) error {
	for _, q := range []string{
		`DELETE FROM cycle_edge WHERE dimension_id = ?`,
		`DELETE FROM cycle_member WHERE dimension_id = ?`,
		`DELETE FROM cycle_group WHERE dimension_id = ?`,
	} {
		if _, err := r.db().Exec(q, dimID); err != nil {
			return err
		}
	}
	return nil
}

// CycleMemberRow 环组成员写入行。
type CycleMemberRow struct {
	GroupID     int
	TypeQN      string
	NamespaceQN string
}

// CycleEdgeRow 环组边写入行。
type CycleEdgeRow struct {
	GroupID  int
	SourceQN string
	TargetQN string
	Types    []string
	Sites    int
}

// InsertCycleGroup 写入一个环组及其成员/边（单事务）。
func (r *Repository) InsertCycleGroup(dimID int, groupID, memberCount, pkgCount int, members []CycleMemberRow, edges []CycleEdgeRow) error {
	c := r.db()
	tx, err := c.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`INSERT INTO cycle_group (dimension_id, group_id, member_count, pkg_count) VALUES (?,?,?,?)`,
		dimID, groupID, memberCount, pkgCount); err != nil {
		return err
	}
	for _, m := range members {
		if _, err := tx.Exec(`INSERT INTO cycle_member (dimension_id, group_id, type_qn, namespace_qn) VALUES (?,?,?,?)`,
			dimID, m.GroupID, m.TypeQN, m.NamespaceQN); err != nil {
			return err
		}
	}
	for _, e := range edges {
		if _, err := tx.Exec(`INSERT INTO cycle_edge (dimension_id, group_id, source_qn, target_qn, types, site_count)
			VALUES (?,?,?,?,?,?)`, dimID, e.GroupID, e.SourceQN, e.TargetQN, arrayLiteral(e.Types), e.Sites); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// CycleMembersByGroup 返回 group_id → 成员类型 QN 列表（包级上卷用）。
func (r *Repository) CycleMembersByGroup(dimID int) (map[int][]string, error) {
	rows, err := r.db().Query(`SELECT group_id, type_qn FROM cycle_member WHERE dimension_id = ?`, dimID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int][]string{}
	for rows.Next() {
		var gid int
		var qn string
		if err := rows.Scan(&gid, &qn); err != nil {
			return nil, err
		}
		out[gid] = append(out[gid], qn)
	}
	return out, rows.Err()
}

type CycleGroup struct {
	ID          int
	MemberCount int
	PkgCount    int
	Members     []string
	Pkgs        []string
	Hops        []CycleHop
}

type CycleHop struct {
	Source string
	Target string
	Types  []string
	Sites  int
}

func (r *Repository) CycleGroups(dimID int) ([]CycleGroup, error) {
	rows, err := r.db().Query(`SELECT group_id, member_count, pkg_count FROM cycle_group
		WHERE dimension_id = ? ORDER BY group_id`, dimID)
	if err != nil {
		return nil, err
	}
	var groups []CycleGroup
	idx := map[int]int{}
	for rows.Next() {
		var g CycleGroup
		if err := rows.Scan(&g.ID, &g.MemberCount, &g.PkgCount); err != nil {
			rows.Close()
			return nil, err
		}
		idx[g.ID] = len(groups)
		groups = append(groups, g)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(groups) == 0 {
		return groups, nil
	}

	mrows, err := r.db().Query(`SELECT group_id, type_qn, COALESCE(namespace_qn,'') FROM cycle_member
		WHERE dimension_id = ? ORDER BY group_id, type_qn`, dimID)
	if err != nil {
		return nil, err
	}
	for mrows.Next() {
		var gid int
		var qn, ns string
		if err := mrows.Scan(&gid, &qn, &ns); err != nil {
			mrows.Close()
			return nil, err
		}
		if i, ok := idx[gid]; ok {
			groups[i].Members = append(groups[i].Members, qn)
			groups[i].Pkgs = append(groups[i].Pkgs, ns)
		}
	}
	mrows.Close()

	erows, err := r.db().Query(`SELECT group_id, source_qn, target_qn, CAST(types AS VARCHAR), site_count FROM cycle_edge
		WHERE dimension_id = ? ORDER BY group_id, source_qn, target_qn`, dimID)
	if err != nil {
		return nil, err
	}
	for erows.Next() {
		var gid int
		var h CycleHop
		var typesStr string
		if err := erows.Scan(&gid, &h.Source, &h.Target, &typesStr, &h.Sites); err != nil {
			erows.Close()
			return nil, err
		}
		h.Types = SplitArrayString(typesStr)
		if i, ok := idx[gid]; ok {
			groups[i].Hops = append(groups[i].Hops, h)
		}
	}
	erows.Close()
	return groups, erows.Err()
}

type SccComp struct {
	ID      int
	Members []string
}

func (r *Repository) SCC(dimID int) ([]SccComp, error) {
	rows, err := r.db().Query(`SELECT group_id, type_qn FROM cycle_member WHERE dimension_id = ? ORDER BY group_id, type_qn`, dimID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	compMap := map[int]*SccComp{}
	var order []int
	for rows.Next() {
		var cid int
		var m string
		if err := rows.Scan(&cid, &m); err != nil {
			return nil, err
		}
		if _, ok := compMap[cid]; !ok {
			compMap[cid] = &SccComp{ID: cid}
			order = append(order, cid)
		}
		compMap[cid].Members = append(compMap[cid].Members, m)
	}
	out := make([]SccComp, 0, len(order))
	for _, cid := range order {
		out = append(out, *compMap[cid])
	}
	return out, rows.Err()
}
