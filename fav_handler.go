package v1

import (
	"douyin/service"

	"github.com/gin-gonic/gin"
	"github.com/gomodule/redigo/redis"
)

const (
	successCode = 0
	errorCode   = 1
)

func Response(ctx *gin.Context, httpStatus int, v interface{}) {
	ctx.JSON(httpStatus, v)
}

type FavActionParams struct {
	Token      string `form:"token" binding:"required"`
	VideoId    int64  `form:"video_id" binding:"required"`
	ActionType int8   `form:"action_type" binding:"required,oneof=1 2"`
}

// 点赞视频
func FavoriteAction(ctx *gin.Context) {
	var favInfo FavActionParams
	err := ctx.ShouldBind(&favInfo)
	if err != nil {
		Response(ctx, 400, gin.H{"error": err.Error()})
		return
	}
	tokenUids, _ := ctx.Get("UserId")
	tokenUid, _ := tokenUids.(int64)

	if err != nil {
		Response(ctx, 500, gin.H{"error": err.Error()})
		return
	}

	redisPool, _ := ctx.Get("RedisPool")
	err = service.FavoriteAction(tokenUid, favInfo.VideoId, favInfo.ActionType, redisPool.(*redis.Pool))

	if err != nil {
		Response(ctx, 500, gin.H{"error": err.Error()})
		return
	}
	Response(ctx, 200, gin.H{"message": "success"})
	// 获取 Redis 连接池

}
