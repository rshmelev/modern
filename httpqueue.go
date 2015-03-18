package modern

import (
	"errors"
	"time"

	gq "github.com/rshmelev/goquse"
)

type IHttpQueues interface {
	gq.IQueues
	PutRequest(qname string, url string, reshandler HttpQueueResultsHandler, MaxExecutionTime time.Duration) gq.BasicQueueItem
}

type HttpQueueResultsHandler func(task *HttpQueueTask, info HttpQueueExecutionResult, response *HttpByteResponse)
type HttpQueueExecutionResult interface {
	gq.QueueExecutionResult
}

type HttpQueues struct {
	gq.Queues
}
type HttpQueueTask struct {
	Url string
}

func MakeCoreHandlerFromHttpQueueHandler(reshandler HttpQueueResultsHandler) gq.QueueResultHandlerFunc {
	var thehandler gq.QueueResultHandlerFunc
	if reshandler != nil {
		thehandler = func(item gq.QueueExecutionResult) {
			br, _ := item.GetResult().(*HttpByteResponse)
			//fmt.Println(item.GetError(), br.ToStringOrError())
			t, _ := item.GetTask().(*HttpQueueTask)
			titem, _ := item.(HttpQueueExecutionResult)
			reshandler(t, titem, br)
		}
	}
	return thehandler
}

func (q *HttpQueues) PutRequest(qname string, url string, reshandler HttpQueueResultsHandler, MaxExecutionTime time.Duration) gq.BasicQueueItem {
	t := &HttpQueueTask{Url: url} //, resHandler: reshandler}
	thehandler := MakeCoreHandlerFromHttpQueueHandler(reshandler)
	i := q.MakeQueueItem(qname, t, MaxExecutionTime, thehandler)
	q.PutAsync(i)

	return i
}

func (q *HttpQueues) httpRequestor(item gq.QueueTaskWrapper) error {
	t, ok := item.GetTask().(*HttpQueueTask)
	if !ok {
		return errors.New("omg not an url")
	}

	x := F.GetByteContents(t.Url, item.GetMaxExecutionTime())
	item.SetResult(x)

	return x.Err
}

func CreateHttpQueues(handler HttpQueueResultsHandler) IHttpQueues {
	thehandler := MakeCoreHandlerFromHttpQueueHandler(handler)
	internalqueues, ok := gq.CreateQueues(nil, thehandler, nil).(*gq.Queues)
	if !ok {
		panic("sfdfsdf")
	}
	q := &HttpQueues{
		*internalqueues,
	}
	q.Queues.SetExecutor(q.httpRequestor)
	return q
}
