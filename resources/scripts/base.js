window.onresize=function(){setSizes()};
window.onload=function(){setSizes()};

function setSizes(){
	document.getElementById('user_content').style.height = 
		((document.documentElement.clientHeight - 170) + "px");
}


maxL=255;
function Limit(Obj) {
	if (Obj.value.length == maxL) {
		return false;
	}
	return true;
}

function Count(Obj, Count) {
	objCnt = createObject(Count);
	objVal = Obj.value;
	if (objVal.length > maxL) {
		objVal = objVal.substring(0, maxL);
	}
	if (objCnt) {
		if (navigator.appName == "Netscape") {	
			objCnt.textContent = objVal.length + "/" + maxL;
		} else {
			objCnt.innerText = objVal.length + "/" + maxL;
		}
	}
	return true;
}

function createObject(objId) {
	if (document.getElementById) return document.getElementById(objId);
	else if (document.layers) return eval("document." + objId);
	else if (document.all) return eval("document.all." + objId);
	else return eval("document." + objId);
}
