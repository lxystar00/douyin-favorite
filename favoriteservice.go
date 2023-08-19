package service

import (
	"douyin/dao"
	"strconv"

	"github.com/gomodule/redigo/redis"
)

func FavoriteAction(uid, vid int64, action int8, redisPool *redis.Pool) error {
	if action == 1 {
		dao.FavoriteTableChange("favorite", uid, vid, true)
		dao.ChangeUserFavoriteCount("user", uid, true)
		dao.ChangeVideoLikesCount("video", vid, true)
		CacheFavoriteAction(uid, vid, true, redisPool)
	} else {
		dao.FavoriteTableChange("favorite", uid, vid, false)
		dao.ChangeUserFavoriteCount("user", uid, false)
		dao.ChangeVideoLikesCount("video", vid, false)
		CacheFavoriteAction(uid, vid, false, redisPool)

	}

	return nil
}

func CacheFavoriteAction(uid, vid int64, action bool, redisPool *redis.Pool) error {
	conn := redisPool.Get() //重用已有的连接
	defer conn.Close()

	//使用 Redis 的字符串（String）来存储用户表和视频表的信息，使用集合（Set）来存储点赞关系。

	if action {
		conn.Send("SADD", "user:"+strconv.FormatInt(uid, 10)+":likes", vid)
		conn.Send("INCR", "user:"+strconv.FormatInt(uid, 10)+":favorite_count")
		conn.Send("INCR", "video:"+strconv.FormatInt(vid, 10)+":likes_count")

	} else {
		conn.Send("SREM", "user:"+strconv.FormatInt(uid, 10)+":likes", vid)
		conn.Send("DECR", "user:"+strconv.FormatInt(uid, 10)+":favorite_count")
		conn.Send("DECR", "video:"+strconv.FormatInt(vid, 10)+":likes_count")
	}
	conn.Flush()
	return nil
}
